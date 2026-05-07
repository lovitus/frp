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
	sscore "github.com/shadowsocks/go-shadowsocks2/core"
)

const (
	gatewayTunnelNameMaxLen   = 64
	gatewayTunnelRemarkMaxLen = 256
	gatewayStatusTimeout      = 2 * time.Second
	gatewayValidityMaxValue   = 3650
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

	expiryChangedCh chan struct{}

	lookupClient func(string) (registry.ClientInfo, bool)
	sendMessage  func(string, msg.Message) error
}

func NewGatewayTunnelManager(
	lookupClient func(string) (registry.ClientInfo, bool),
	sendMessage func(string, msg.Message) error,
) *GatewayTunnelManager {
	return &GatewayTunnelManager{
		tunnels:         make(map[string]*GatewayTunnel),
		waiters:         make(map[string]*gatewayStatusWaiter),
		lookupClient:    lookupClient,
		sendMessage:     sendMessage,
		expiryChangedCh: make(chan struct{}, 1),
	}
}

func (m *GatewayTunnelManager) List() []GatewayTunnel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]GatewayTunnel, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		items = append(items, applyExpiredState(cloneGatewayTunnel(tunnel)))
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
	return applyExpiredState(cloneGatewayTunnel(tunnel)), true
}

func (m *GatewayTunnelManager) Create(tunnel GatewayTunnel) (GatewayTunnel, error) {
	normalized, err := normalizeGatewayTunnel(tunnel, true, nil)
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
	m.notifyExpiryChanged()
	return cloneGatewayTunnel(&normalized), nil
}

func (m *GatewayTunnelManager) Update(id string, tunnel GatewayTunnel) (GatewayTunnel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, ok := m.tunnels[id]
	if !ok {
		return GatewayTunnel{}, fmt.Errorf("gateway tunnel %q not found", id)
	}
	tunnel.ID = id
	normalized, err := normalizeGatewayTunnel(tunnel, false, current)
	if err != nil {
		return GatewayTunnel{}, err
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
	current.TargetType = normalized.TargetType
	current.SSMethod = normalized.SSMethod
	current.SSPassword = normalized.SSPassword
	current.UOTEnabled = normalized.UOTEnabled
	current.UOTVersion = normalized.UOTVersion
	current.Socks5Auth = normalized.Socks5Auth
	current.Socks5User = normalized.Socks5User
	current.Socks5Pass = normalized.Socks5Pass
	current.ValidityValue = normalized.ValidityValue
	current.ValidityUnit = normalized.ValidityUnit
	current.ExpiresAt = normalized.ExpiresAt
	current.Status = gatewaypkg.StatusPending
	current.Message = ""
	current.RemoteAddr = ""
	current.UpdatedAt = time.Now()
	m.notifyExpiryChanged()
	return cloneGatewayTunnel(current), nil
}

func (m *GatewayTunnelManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tunnels[id]; !ok {
		return fmt.Errorf("gateway tunnel %q not found", id)
	}
	delete(m.tunnels, id)
	m.notifyExpiryChanged()
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

func (m *GatewayTunnelManager) ExpireDue(now time.Time) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	affected := map[string]struct{}{}
	for _, tunnel := range m.tunnels {
		if !isTunnelExpired(tunnel, now) {
			continue
		}
		if tunnel.Status != gatewaypkg.StatusExpired || tunnel.Message == "" {
			tunnel.Status = gatewaypkg.StatusExpired
			tunnel.Message = buildGatewayValidityStatus(tunnel)
			tunnel.RemoteAddr = ""
			tunnel.UpdatedAt = now
			affected[tunnel.ClientKey] = struct{}{}
		}
	}
	if len(affected) == 0 {
		return nil
	}
	keys := make([]string, 0, len(affected))
	for key := range affected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (m *GatewayTunnelManager) RefreshStatus(ctx context.Context, tunnelIDs []string) {
	grouped := m.groupTunnelIDsByClient(tunnelIDs)
	if len(grouped) == 0 {
		return
	}

	var wg sync.WaitGroup
	for clientKey, ids := range grouped {
		activeIDs, expiredChanged := m.markExpiredAndCollectActiveIDs(clientKey, ids)
		if len(activeIDs) == 0 {
			if expiredChanged {
				if clientInfo, ok := m.lookupClient(clientKey); ok && clientInfo.Online && clientInfo.AllowGatewayTunnels {
					_ = m.SyncClient(clientKey)
				}
			}
			continue
		}

		clientInfo, ok := m.lookupClient(clientKey)
		if !ok || !clientInfo.Online {
			m.setClientStatus(clientKey, activeIDs, gatewaypkg.StatusClientOffline, "client is offline")
			continue
		}
		if !clientInfo.AllowGatewayTunnels {
			m.setClientStatus(clientKey, activeIDs, gatewaypkg.StatusDisabled, "client does not allow gateway tunnels")
			continue
		}
		if expiredChanged {
			_ = m.SyncClient(clientKey)
		}
		activeIDs, unsupportedIDs := m.partitionUnsupportedSingSSIDs(clientInfo, clientKey, activeIDs)
		if len(unsupportedIDs) > 0 {
			m.setClientStatus(clientKey, unsupportedIDs, gatewaypkg.StatusClientUnsupported, "client does not support sing_ss_proxy")
			_ = m.SyncClient(clientKey)
		}
		if len(activeIDs) == 0 {
			continue
		}

		wg.Add(1)
		go func(clientKey string, ids []string) {
			defer wg.Done()
			m.requestClientStatus(ctx, clientKey, ids)
		}(clientKey, activeIDs)
	}
	wg.Wait()
}

func (m *GatewayTunnelManager) HandleClientConnected(clientKey string) {
	if clientInfo, ok := m.lookupClient(clientKey); ok && clientInfo.Online && clientInfo.AllowGatewayTunnels {
		supportedIDs, unsupportedIDs := m.partitionUnsupportedSingSSIDs(clientInfo, clientKey, nil)
		if len(supportedIDs) > 0 {
			m.setClientStatus(clientKey, supportedIDs, gatewaypkg.StatusPending, "")
		}
		if len(unsupportedIDs) > 0 {
			m.setClientStatus(clientKey, unsupportedIDs, gatewaypkg.StatusClientUnsupported, "client does not support sing_ss_proxy")
		}
	}
	if err := m.SyncClient(clientKey); err != nil {
		m.setClientStatus(clientKey, nil, gatewaypkg.StatusPending, err.Error())
	}
}

func (m *GatewayTunnelManager) NextExpiry(now time.Time) (time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var next time.Time
	for _, tunnel := range m.tunnels {
		if tunnel.ExpiresAt.IsZero() || !tunnel.ExpiresAt.After(now) {
			continue
		}
		if next.IsZero() || tunnel.ExpiresAt.Before(next) {
			next = tunnel.ExpiresAt
		}
	}
	if next.IsZero() {
		return time.Time{}, false
	}
	return next, true
}

func (m *GatewayTunnelManager) ExpiryChanged() <-chan struct{} {
	return m.expiryChangedCh
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
	clientInfo, hasClientInfo := m.lookupClient(clientKey)
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]msg.GatewayTunnelConfig, 0)
	for _, tunnel := range m.tunnels {
		if tunnel.ClientKey != clientKey {
			continue
		}
		if isTunnelExpired(tunnel, time.Now()) {
			continue
		}
		if tunnel.TargetType == gatewaypkg.TargetTypeSingSSProxy &&
			hasClientInfo && !clientSupportsGatewaySingSSProxy(clientInfo) {
			continue
		}
		items = append(items, msg.GatewayTunnelConfig{
			ID:         tunnel.ID,
			Name:       tunnel.Name,
			Remark:     tunnel.Remark,
			Protocol:   tunnel.Protocol,
			BindAddr:   tunnel.BindAddr,
			ListenPort: tunnel.ListenPort,
			TargetType: tunnel.TargetType,
			TargetHost: tunnel.TargetHost,
			TargetPort: tunnel.TargetPort,
			SSMethod:   tunnel.SSMethod,
			SSPassword: tunnel.SSPassword,
			UOTEnabled: tunnel.UOTEnabled,
			UOTVersion: tunnel.UOTVersion,
			Socks5Auth: tunnel.Socks5Auth,
			Socks5User: tunnel.Socks5User,
			Socks5Pass: tunnel.Socks5Pass,
			ExpiresAt:  toUnixTime(tunnel.ExpiresAt),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func (m *GatewayTunnelManager) partitionUnsupportedSingSSIDs(
	clientInfo registry.ClientInfo,
	clientKey string,
	ids []string,
) ([]string, []string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	supported := make([]string, 0)
	unsupported := make([]string, 0)
	supportsSingSS := clientSupportsGatewaySingSSProxy(clientInfo)
	for _, tunnel := range m.tunnels {
		if tunnel.ClientKey != clientKey {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[tunnel.ID]; !ok {
				continue
			}
		}
		if isTunnelExpired(tunnel, time.Now()) {
			continue
		}
		if tunnel.TargetType == gatewaypkg.TargetTypeSingSSProxy && !supportsSingSS {
			unsupported = append(unsupported, tunnel.ID)
			continue
		}
		supported = append(supported, tunnel.ID)
	}
	return supported, unsupported
}

func clientSupportsGatewaySingSSProxy(clientInfo registry.ClientInfo) bool {
	return clientInfo.Metas[gatewaypkg.CapabilityGatewaySingSSProxy] == "true"
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

func (m *GatewayTunnelManager) markExpiredAndCollectActiveIDs(clientKey string, ids []string) ([]string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	activeIDs := make([]string, 0)
	expiredChanged := false
	for _, tunnel := range m.tunnels {
		if tunnel.ClientKey != clientKey {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[tunnel.ID]; !ok {
				continue
			}
		}
		if !isTunnelExpired(tunnel, now) {
			activeIDs = append(activeIDs, tunnel.ID)
			continue
		}
		if tunnel.Status != gatewaypkg.StatusExpired || tunnel.Message == "" {
			tunnel.Status = gatewaypkg.StatusExpired
			tunnel.Message = buildGatewayValidityStatus(tunnel)
			tunnel.RemoteAddr = ""
			tunnel.UpdatedAt = now
			expiredChanged = true
		}
	}
	return activeIDs, expiredChanged
}

func (m *GatewayTunnelManager) notifyExpiryChanged() {
	select {
	case m.expiryChangedCh <- struct{}{}:
	default:
	}
}

func normalizeGatewayTunnel(tunnel GatewayTunnel, isCreate bool, current *GatewayTunnel) (GatewayTunnel, error) {
	tunnel.ID = strings.TrimSpace(tunnel.ID)
	tunnel.Name = strings.TrimSpace(tunnel.Name)
	tunnel.Remark = strings.TrimSpace(tunnel.Remark)
	tunnel.Protocol = strings.ToLower(strings.TrimSpace(tunnel.Protocol))
	tunnel.BindAddr = strings.TrimSpace(tunnel.BindAddr)
	tunnel.ClientKey = strings.TrimSpace(tunnel.ClientKey)
	tunnel.TargetType = strings.ToLower(strings.TrimSpace(tunnel.TargetType))
	tunnel.TargetHost = strings.TrimSpace(tunnel.TargetHost)
	tunnel.SSMethod = strings.TrimSpace(tunnel.SSMethod)
	tunnel.SSPassword = strings.TrimSpace(tunnel.SSPassword)
	tunnel.Socks5User = strings.TrimSpace(tunnel.Socks5User)
	tunnel.Socks5Pass = strings.TrimSpace(tunnel.Socks5Pass)
	tunnel.ValidityUnit = strings.ToLower(strings.TrimSpace(tunnel.ValidityUnit))
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
	if tunnel.TargetType == "" {
		tunnel.TargetType = gatewaypkg.TargetTypeDirect
	}
	switch tunnel.TargetType {
	case gatewaypkg.TargetTypeDirect:
		tunnel.SSMethod = ""
		tunnel.SSPassword = ""
		tunnel.UOTEnabled = false
		tunnel.UOTVersion = 0
		tunnel.Socks5Auth = false
		tunnel.Socks5User = ""
		tunnel.Socks5Pass = ""
		if tunnel.TargetHost == "" {
			tunnel.TargetHost = "127.0.0.1"
		}
		if tunnel.TargetPort <= 0 || tunnel.TargetPort > 65535 {
			return tunnel, fmt.Errorf("targetPort must be between 1 and 65535")
		}
		if err := validateGatewayTargetHost(tunnel.TargetHost); err != nil {
			return tunnel, err
		}
	case gatewaypkg.TargetTypeSSProxy:
		tunnel.TargetHost = ""
		tunnel.TargetPort = 0
		tunnel.UOTEnabled = false
		tunnel.UOTVersion = 0
		tunnel.Socks5Auth = false
		tunnel.Socks5User = ""
		tunnel.Socks5Pass = ""
		if current != nil && current.TargetType == gatewaypkg.TargetTypeSSProxy && tunnel.SSPassword == "" {
			tunnel.SSPassword = current.SSPassword
		}
		if tunnel.SSMethod == "" {
			return tunnel, fmt.Errorf("ssMethod is required for ss_proxy")
		}
		if tunnel.SSPassword == "" {
			return tunnel, fmt.Errorf("ssPassword is required for ss_proxy")
		}
		if err := validateGatewaySSMethod(tunnel.Protocol, tunnel.SSMethod, tunnel.SSPassword); err != nil {
			return tunnel, err
		}
	case gatewaypkg.TargetTypeSingSSProxy:
		tunnel.TargetHost = ""
		tunnel.TargetPort = 0
		tunnel.Socks5Auth = false
		tunnel.Socks5User = ""
		tunnel.Socks5Pass = ""
		if current != nil && current.TargetType == gatewaypkg.TargetTypeSingSSProxy && tunnel.SSPassword == "" {
			tunnel.SSPassword = current.SSPassword
		}
		if tunnel.SSMethod == "" {
			return tunnel, fmt.Errorf("ssMethod is required for sing_ss_proxy")
		}
		if tunnel.SSPassword == "" {
			return tunnel, fmt.Errorf("ssPassword is required for sing_ss_proxy")
		}
		if tunnel.Protocol == "udp" {
			tunnel.UOTEnabled = false
			tunnel.UOTVersion = 0
		} else if tunnel.UOTEnabled {
			if tunnel.UOTVersion == 0 {
				tunnel.UOTVersion = 2
			}
			if tunnel.UOTVersion != 1 && tunnel.UOTVersion != 2 {
				return tunnel, fmt.Errorf("uotVersion must be 1 or 2")
			}
		} else {
			tunnel.UOTVersion = 0
		}
		if err := gatewaypkg.ValidateGatewaySingSSMethod(tunnel.Protocol, tunnel.SSMethod, tunnel.SSPassword); err != nil {
			return tunnel, err
		}
	case gatewaypkg.TargetTypeSocks5Proxy:
		tunnel.TargetHost = ""
		tunnel.TargetPort = 0
		tunnel.SSMethod = ""
		tunnel.SSPassword = ""
		tunnel.UOTEnabled = false
		tunnel.UOTVersion = 0
		if current != nil && current.TargetType == gatewaypkg.TargetTypeSocks5Proxy {
			if tunnel.Socks5User == "" {
				tunnel.Socks5User = current.Socks5User
			}
			if tunnel.Socks5Pass == "" {
				tunnel.Socks5Pass = current.Socks5Pass
			}
		}
		if tunnel.Protocol != "tcp" {
			return tunnel, fmt.Errorf("socks5_proxy currently supports tcp only")
		}
		if !tunnel.Socks5Auth {
			tunnel.Socks5User = ""
			tunnel.Socks5Pass = ""
		}
		if tunnel.Socks5Auth && (tunnel.Socks5User == "" || tunnel.Socks5Pass == "") {
			return tunnel, fmt.Errorf("socks5 username and password are required when auth is enabled")
		}
	default:
		return tunnel, fmt.Errorf("unsupported targetType %q", tunnel.TargetType)
	}
	switch tunnel.ValidityUnit {
	case "", gatewaypkg.ValidityUnitPermanent:
		tunnel.ValidityUnit = gatewaypkg.ValidityUnitPermanent
		tunnel.ValidityValue = 0
		tunnel.ExpiresAt = time.Time{}
	case gatewaypkg.ValidityUnitHour, gatewaypkg.ValidityUnitDay:
		if tunnel.ValidityValue <= 0 || tunnel.ValidityValue > gatewayValidityMaxValue {
			return tunnel, fmt.Errorf("validityValue must be between 1 and %d", gatewayValidityMaxValue)
		}
		tunnel.ExpiresAt = resolveGatewayTunnelExpiry(tunnel, current)
	default:
		return tunnel, fmt.Errorf("validityUnit must be one of permanent, h, d")
	}
	return tunnel, nil
}

func resolveGatewayTunnelExpiry(tunnel GatewayTunnel, current *GatewayTunnel) time.Time {
	now := time.Now()
	if !tunnel.ExpiresAt.IsZero() {
		return tunnel.ExpiresAt.UTC()
	}
	if current != nil &&
		current.ValidityUnit == tunnel.ValidityUnit &&
		current.ValidityValue == tunnel.ValidityValue &&
		!current.ExpiresAt.IsZero() &&
		current.ExpiresAt.After(now) {
		return current.ExpiresAt.UTC()
	}
	duration := time.Duration(tunnel.ValidityValue) * time.Hour
	if tunnel.ValidityUnit == gatewaypkg.ValidityUnitDay {
		duration = time.Duration(tunnel.ValidityValue) * 24 * time.Hour
	}
	return now.Add(duration).UTC()
}

func validateGatewaySSMethod(protocol, method, password string) error {
	ciph, err := sscore.PickCipher(method, nil, password)
	if err != nil {
		return fmt.Errorf("invalid ssMethod %q: %w", method, err)
	}
	if protocol == "udp" {
		if _, ok := ciph.(sscore.PacketConnCipher); !ok {
			return fmt.Errorf("ssMethod %q does not support udp", method)
		}
	}
	return nil
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

func isTunnelExpired(tunnel *GatewayTunnel, now time.Time) bool {
	if tunnel == nil || tunnel.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(tunnel.ExpiresAt)
}

func applyExpiredState(tunnel GatewayTunnel) GatewayTunnel {
	if isTunnelExpired(&tunnel, time.Now()) {
		tunnel.Status = gatewaypkg.StatusExpired
		tunnel.Message = buildGatewayValidityStatus(&tunnel)
		tunnel.RemoteAddr = ""
	}
	return tunnel
}

func buildGatewayValidityStatus(tunnel *GatewayTunnel) string {
	if tunnel == nil {
		return ""
	}
	if tunnel.ValidityUnit == gatewaypkg.ValidityUnitPermanent || tunnel.ExpiresAt.IsZero() {
		return "permanent"
	}
	return fmt.Sprintf("valid until %s", tunnel.ExpiresAt.Local().Format("2006-01-02 15:04:05"))
}

func toUnixTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
