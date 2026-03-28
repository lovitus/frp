package server

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	gatewaypkg "github.com/fatedier/frp/pkg/gateway"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/util/util"
	"github.com/fatedier/frp/server/registry"
)

const (
	gatewayTunnelNameMaxLen   = 64
	gatewayTunnelRemarkMaxLen = 256
	gatewayStatusTimeout      = 2 * time.Second
)

type GatewayTunnel = gatewaypkg.Tunnel

type gatewayStatusWaiter struct {
	clientKey string
	ch        chan []msg.GatewayTunnelStatus
}

type GatewayTunnelManager struct {
	mu sync.RWMutex

	tunnels map[string]*GatewayTunnel
	waiters map[string]*gatewayStatusWaiter

	lookupClient func(string) (registry.ClientInfo, bool)
	sendMessage  func(string, msg.Message) error
}

func NewGatewayTunnelManager(
	lookupClient func(string) (registry.ClientInfo, bool),
	sendMessage func(string, msg.Message) error,
) *GatewayTunnelManager {
	return &GatewayTunnelManager{
		tunnels:      make(map[string]*GatewayTunnel),
		waiters:      make(map[string]*gatewayStatusWaiter),
		lookupClient: lookupClient,
		sendMessage:  sendMessage,
	}
}

func (m *GatewayTunnelManager) List() []GatewayTunnel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]GatewayTunnel, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		items = append(items, cloneGatewayTunnel(tunnel))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ClientKey != items[j].ClientKey {
			return items[i].ClientKey < items[j].ClientKey
		}
		return items[i].Name < items[j].Name
	})
	return items
}

func (m *GatewayTunnelManager) Get(id string) (GatewayTunnel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tunnel, ok := m.tunnels[id]
	if !ok {
		return GatewayTunnel{}, false
	}
	return cloneGatewayTunnel(tunnel), true
}

func (m *GatewayTunnelManager) Create(tunnel GatewayTunnel) (GatewayTunnel, error) {
	normalized, err := normalizeGatewayTunnel(tunnel, true)
	if err != nil {
		return GatewayTunnel{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, existing := range m.tunnels {
		if existing.ClientKey == normalized.ClientKey && existing.Name == normalized.Name {
			return GatewayTunnel{}, fmt.Errorf("gateway tunnel %q already exists for client %q", normalized.Name, normalized.ClientKey)
		}
	}
	now := time.Now()
	normalized.Status = gatewaypkg.StatusPending
	normalized.UpdatedAt = now
	m.tunnels[normalized.ID] = &normalized
	return cloneGatewayTunnel(&normalized), nil
}

func (m *GatewayTunnelManager) Update(id string, tunnel GatewayTunnel) (GatewayTunnel, error) {
	normalized, err := normalizeGatewayTunnel(tunnel, false)
	if err != nil {
		return GatewayTunnel{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	current, ok := m.tunnels[id]
	if !ok {
		return GatewayTunnel{}, fmt.Errorf("gateway tunnel %q not found", id)
	}
	for existingID, existing := range m.tunnels {
		if existingID == id {
			continue
		}
		if existing.ClientKey == normalized.ClientKey && existing.Name == normalized.Name {
			return GatewayTunnel{}, fmt.Errorf("gateway tunnel %q already exists for client %q", normalized.Name, normalized.ClientKey)
		}
	}

	current.Name = normalized.Name
	current.Remark = normalized.Remark
	current.Protocol = normalized.Protocol
	current.BindAddr = normalized.BindAddr
	current.ListenPort = normalized.ListenPort
	current.ClientKey = normalized.ClientKey
	current.TargetHost = normalized.TargetHost
	current.TargetPort = normalized.TargetPort
	current.Status = gatewaypkg.StatusPending
	current.Message = ""
	current.RemoteAddr = ""
	current.UpdatedAt = time.Now()
	return cloneGatewayTunnel(current), nil
}

func (m *GatewayTunnelManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tunnels[id]; !ok {
		return fmt.Errorf("gateway tunnel %q not found", id)
	}
	delete(m.tunnels, id)
	return nil
}

func (m *GatewayTunnelManager) SyncClient(clientKey string) error {
	if m.sendMessage == nil {
		return fmt.Errorf("gateway tunnel sender is unavailable")
	}
	return m.sendMessage(clientKey, &msg.GatewayTunnelsSync{
		Tunnels: m.listWireConfigsForClient(clientKey),
	})
}

func (m *GatewayTunnelManager) RefreshStatus(ctx context.Context, tunnelIDs []string) {
	grouped := m.groupTunnelIDsByClient(tunnelIDs)
	if len(grouped) == 0 {
		return
	}

	var wg sync.WaitGroup
	for clientKey, ids := range grouped {
		clientInfo, ok := m.lookupClient(clientKey)
		if !ok || !clientInfo.Online {
			m.setClientStatus(clientKey, ids, gatewaypkg.StatusClientOffline, "client is offline")
			continue
		}
		if !clientInfo.AllowGatewayTunnels {
			m.setClientStatus(clientKey, ids, gatewaypkg.StatusDisabled, "client does not allow gateway tunnels")
			continue
		}

		wg.Add(1)
		go func(clientKey string, ids []string) {
			defer wg.Done()
			m.requestClientStatus(ctx, clientKey, ids)
		}(clientKey, ids)
	}
	wg.Wait()
}

func (m *GatewayTunnelManager) HandleClientConnected(clientKey string) {
	if err := m.SyncClient(clientKey); err != nil {
		m.setClientStatus(clientKey, nil, gatewaypkg.StatusPending, err.Error())
	}
}

func (m *GatewayTunnelManager) HandleStatusResponse(clientKey string, resp *msg.GatewayTunnelStatusResponse) {
	if resp == nil || resp.RequestID == "" {
		return
	}

	m.mu.Lock()
	waiter, ok := m.waiters[resp.RequestID]
	if ok {
		delete(m.waiters, resp.RequestID)
	}
	for _, item := range resp.Statuses {
		tunnel, exists := m.tunnels[item.ID]
		if !exists || tunnel.ClientKey != clientKey {
			continue
		}
		tunnel.Status = item.Status
		tunnel.Message = item.Message
		tunnel.RemoteAddr = item.RemoteAddr
		if item.UpdatedAt > 0 {
			tunnel.UpdatedAt = time.Unix(item.UpdatedAt, 0)
		} else {
			tunnel.UpdatedAt = time.Now()
		}
	}
	m.mu.Unlock()

	if ok && waiter.clientKey == clientKey {
		select {
		case waiter.ch <- resp.Statuses:
		default:
		}
	}
}

func (m *GatewayTunnelManager) requestClientStatus(ctx context.Context, clientKey string, ids []string) {
	if m.sendMessage == nil {
		m.setClientStatus(clientKey, ids, gatewaypkg.StatusPending, "status request sender is unavailable")
		return
	}

	requestID, err := util.RandID()
	if err != nil {
		m.setClientStatus(clientKey, ids, gatewaypkg.StatusPending, err.Error())
		return
	}
	waiter := &gatewayStatusWaiter{
		clientKey: clientKey,
		ch:        make(chan []msg.GatewayTunnelStatus, 1),
	}

	m.mu.Lock()
	m.waiters[requestID] = waiter
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.waiters, requestID)
		m.mu.Unlock()
	}()

	if err := m.sendMessage(clientKey, &msg.GatewayTunnelStatusRequest{
		RequestID: requestID,
		TunnelIDs: ids,
	}); err != nil {
		m.setClientStatus(clientKey, ids, gatewaypkg.StatusPending, err.Error())
		return
	}

	waitCtx, cancel := context.WithTimeout(ctx, gatewayStatusTimeout)
	defer cancel()

	select {
	case <-waiter.ch:
	case <-waitCtx.Done():
		m.setClientStatus(clientKey, ids, gatewaypkg.StatusPending, "status request timed out")
	}
}

func (m *GatewayTunnelManager) groupTunnelIDsByClient(tunnelIDs []string) map[string][]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]string)
	if len(tunnelIDs) == 0 {
		for id, tunnel := range m.tunnels {
			result[tunnel.ClientKey] = append(result[tunnel.ClientKey], id)
		}
		return result
	}

	for _, id := range tunnelIDs {
		if tunnel, ok := m.tunnels[id]; ok {
			result[tunnel.ClientKey] = append(result[tunnel.ClientKey], id)
		}
	}
	return result
}

func (m *GatewayTunnelManager) listWireConfigsForClient(clientKey string) []msg.GatewayTunnelConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]msg.GatewayTunnelConfig, 0)
	for _, tunnel := range m.tunnels {
		if tunnel.ClientKey != clientKey {
			continue
		}
		items = append(items, msg.GatewayTunnelConfig{
			ID:         tunnel.ID,
			Name:       tunnel.Name,
			Remark:     tunnel.Remark,
			Protocol:   tunnel.Protocol,
			BindAddr:   tunnel.BindAddr,
			ListenPort: tunnel.ListenPort,
			TargetHost: tunnel.TargetHost,
			TargetPort: tunnel.TargetPort,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func (m *GatewayTunnelManager) setClientStatus(clientKey string, ids []string, status, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	for _, tunnel := range m.tunnels {
		if tunnel.ClientKey != clientKey {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[tunnel.ID]; !ok {
				continue
			}
		}
		tunnel.Status = status
		tunnel.Message = message
		tunnel.RemoteAddr = ""
		tunnel.UpdatedAt = time.Now()
	}
}

func normalizeGatewayTunnel(tunnel GatewayTunnel, isCreate bool) (GatewayTunnel, error) {
	tunnel.ID = strings.TrimSpace(tunnel.ID)
	tunnel.Name = strings.TrimSpace(tunnel.Name)
	tunnel.Remark = strings.TrimSpace(tunnel.Remark)
	tunnel.Protocol = strings.ToLower(strings.TrimSpace(tunnel.Protocol))
	tunnel.BindAddr = strings.TrimSpace(tunnel.BindAddr)
	tunnel.ClientKey = strings.TrimSpace(tunnel.ClientKey)
	tunnel.TargetHost = strings.TrimSpace(tunnel.TargetHost)
	if tunnel.TargetHost == "" {
		tunnel.TargetHost = "127.0.0.1"
	}
	if isCreate {
		if tunnel.ID == "" {
			id, err := util.RandID()
			if err != nil {
				return tunnel, err
			}
			tunnel.ID = id
		}
	} else if tunnel.ID == "" {
		return tunnel, fmt.Errorf("gateway tunnel id is required")
	}
	if tunnel.Name == "" {
		return tunnel, fmt.Errorf("name is required")
	}
	if len(tunnel.Name) > gatewayTunnelNameMaxLen {
		return tunnel, fmt.Errorf("name is too long")
	}
	if len(tunnel.Remark) > gatewayTunnelRemarkMaxLen {
		return tunnel, fmt.Errorf("remark is too long")
	}
	if tunnel.Protocol != "tcp" && tunnel.Protocol != "udp" {
		return tunnel, fmt.Errorf("protocol must be tcp or udp")
	}
	if tunnel.BindAddr == "" {
		tunnel.BindAddr = "0.0.0.0"
	}
	if err := validateGatewayBindAddr(tunnel.BindAddr); err != nil {
		return tunnel, err
	}
	if tunnel.ListenPort <= 0 || tunnel.ListenPort > 65535 {
		return tunnel, fmt.Errorf("listenPort must be between 1 and 65535")
	}
	if tunnel.ClientKey == "" {
		return tunnel, fmt.Errorf("clientKey is required")
	}
	if tunnel.TargetPort <= 0 || tunnel.TargetPort > 65535 {
		return tunnel, fmt.Errorf("targetPort must be between 1 and 65535")
	}
	if err := validateGatewayTargetHost(tunnel.TargetHost); err != nil {
		return tunnel, err
	}
	return tunnel, nil
}

func validateGatewayBindAddr(value string) error {
	raw := strings.TrimSpace(strings.Trim(value, "[]"))
	if raw == "" {
		return fmt.Errorf("bindAddr is required")
	}
	if ip := net.ParseIP(raw); ip == nil {
		return fmt.Errorf("bindAddr must be an IP address")
	}
	return nil
}

func validateGatewayTargetHost(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("targetHost is required")
	}
	if len(value) > 255 {
		return fmt.Errorf("targetHost is too long")
	}
	if strings.ContainsAny(value, "\r\n\t") {
		return fmt.Errorf("targetHost contains invalid whitespace")
	}
	return nil
}

func cloneGatewayTunnel(tunnel *GatewayTunnel) GatewayTunnel {
	if tunnel == nil {
		return GatewayTunnel{}
	}
	return *tunnel
}
