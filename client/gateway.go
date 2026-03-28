package client

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fatedier/frp/pkg/config/source"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/config/v1/validation"
	gatewaypkg "github.com/fatedier/frp/pkg/gateway"
	"github.com/fatedier/frp/pkg/msg"
)

type gatewayTunnelRuntime struct {
	config        msg.GatewayTunnelConfig
	proxyName     string
	validationErr string
}

type GatewayTunnelManager struct {
	service       *Service
	runtimeSource *source.ConfigSource
	enabled       bool

	mu           sync.RWMutex
	tunnels      map[string]*gatewayTunnelRuntime
	lastApplyErr string
}

func NewGatewayTunnelManager(service *Service, runtimeSource *source.ConfigSource, enabled bool) *GatewayTunnelManager {
	return &GatewayTunnelManager{
		service:       service,
		runtimeSource: runtimeSource,
		enabled:       enabled,
		tunnels:       make(map[string]*gatewayTunnelRuntime),
	}
}

func (m *GatewayTunnelManager) ApplyGatewayTunnels(tunnels []msg.GatewayTunnelConfig) error {
	next := make(map[string]*gatewayTunnelRuntime, len(tunnels))
	proxyCfgs := make([]v1.ProxyConfigurer, 0, len(tunnels))

	for _, raw := range tunnels {
		cfg, err := normalizeGatewayTunnelConfig(raw)
		if err != nil {
			continue
		}
		runtime := &gatewayTunnelRuntime{
			config:    cfg,
			proxyName: gatewaypkg.ProxyName(cfg.ID),
		}
		if !m.enabled {
			runtime.validationErr = "gateway tunnels are disabled on this client"
			next[cfg.ID] = runtime
			continue
		}

		proxyCfg, err := buildGatewayProxyConfigurer(cfg)
		if err != nil {
			runtime.validationErr = err.Error()
			next[cfg.ID] = runtime
			continue
		}
		next[cfg.ID] = runtime
		proxyCfgs = append(proxyCfgs, proxyCfg)
	}

	if m.runtimeSource == nil {
		return fmt.Errorf("runtime source is unavailable")
	}
	if err := m.runtimeSource.ReplaceAll(proxyCfgs, nil); err != nil {
		return err
	}

	m.mu.Lock()
	m.tunnels = next
	m.lastApplyErr = ""
	m.mu.Unlock()

	if err := m.service.reloadConfigFromSources(); err != nil {
		m.mu.Lock()
		m.lastApplyErr = err.Error()
		m.mu.Unlock()
		return err
	}
	return nil
}

func (m *GatewayTunnelManager) BuildGatewayTunnelStatusResponse(requestID string, tunnelIDs []string) *msg.GatewayTunnelStatusResponse {
	m.mu.RLock()
	applyErr := m.lastApplyErr
	selected := make([]*gatewayTunnelRuntime, 0, len(m.tunnels))
	if len(tunnelIDs) == 0 {
		for _, tunnel := range m.tunnels {
			selected = append(selected, tunnel)
		}
	} else {
		for _, id := range tunnelIDs {
			if tunnel, ok := m.tunnels[id]; ok {
				selected = append(selected, tunnel)
			}
		}
	}
	m.mu.RUnlock()

	sort.Slice(selected, func(i, j int) bool {
		return selected[i].config.Name < selected[j].config.Name
	})

	statuses := make([]msg.GatewayTunnelStatus, 0, len(selected))
	now := time.Now().Unix()
	for _, tunnel := range selected {
		status := msg.GatewayTunnelStatus{
			ID:        tunnel.config.ID,
			Name:      tunnel.config.Name,
			UpdatedAt: now,
		}

		switch {
		case !m.enabled:
			status.Status = gatewaypkg.StatusDisabled
			status.Message = "gateway tunnels are disabled on this client"
		case tunnel.validationErr != "":
			status.Status = gatewaypkg.StatusInvalidConfig
			status.Message = tunnel.validationErr
		default:
			status = m.fillGatewayTunnelRuntimeStatus(status, tunnel, applyErr)
		}
		statuses = append(statuses, status)
	}

	return &msg.GatewayTunnelStatusResponse{
		RequestID: requestID,
		Statuses:  statuses,
	}
}

func (m *GatewayTunnelManager) fillGatewayTunnelRuntimeStatus(
	status msg.GatewayTunnelStatus,
	tunnel *gatewayTunnelRuntime,
	applyErr string,
) msg.GatewayTunnelStatus {
	proxyStatus, ok := m.service.getProxyStatus(tunnel.proxyName)
	if !ok {
		if applyErr != "" {
			status.Status = gatewaypkg.StatusApplyFailed
			status.Message = applyErr
			return status
		}
		status.Status = gatewaypkg.StatusPending
		return status
	}

	status.RemoteAddr = proxyStatus.RemoteAddr
	switch proxyStatus.Phase {
	case "running":
		if err := validateGatewayTunnelTargetReachability(tunnel.config); err != nil {
			var targetErr *gatewayTargetError
			if errors.As(err, &targetErr) {
				status.Status = targetErr.Status
				status.Message = targetErr.Error()
				return status
			}
			status.Status = gatewaypkg.StatusTargetUnreachable
			status.Message = err.Error()
			return status
		}
		status.Status = gatewaypkg.StatusOnline
	case "start error", "check failed":
		status.Status = gatewaypkg.StatusRegisterFailed
		status.Message = proxyStatus.Err
	case "wait start", "new", "closed":
		status.Status = gatewaypkg.StatusPending
		if proxyStatus.Err != "" {
			status.Message = proxyStatus.Err
		}
	default:
		status.Status = gatewaypkg.StatusPending
		if proxyStatus.Err != "" {
			status.Message = proxyStatus.Err
		}
	}
	return status
}

func buildGatewayProxyConfigurer(cfg msg.GatewayTunnelConfig) (v1.ProxyConfigurer, error) {
	base := v1.ProxyBaseConfig{
		Name: gatewaypkg.ProxyName(cfg.ID),
		Type: cfg.Protocol,
		Annotations: map[string]string{
			gatewaypkg.AnnotationSourceKey:   gatewaypkg.AnnotationSourceGatewayTunnel,
			gatewaypkg.AnnotationTunnelIDKey: cfg.ID,
			gatewaypkg.AnnotationBindAddrKey: cfg.BindAddr,
		},
		ProxyBackend: v1.ProxyBackend{
			LocalIP:   cfg.TargetHost,
			LocalPort: cfg.TargetPort,
		},
	}

	switch cfg.Protocol {
	case "tcp":
		proxyCfg := &v1.TCPProxyConfig{
			ProxyBaseConfig: base,
			RemotePort:      cfg.ListenPort,
		}
		proxyCfg.Complete()
		if err := validation.ValidateProxyConfigurerForClient(proxyCfg); err != nil {
			return nil, err
		}
		return proxyCfg, nil
	case "udp":
		proxyCfg := &v1.UDPProxyConfig{
			ProxyBaseConfig: base,
			RemotePort:      cfg.ListenPort,
		}
		proxyCfg.Complete()
		if err := validation.ValidateProxyConfigurerForClient(proxyCfg); err != nil {
			return nil, err
		}
		return proxyCfg, nil
	default:
		return nil, fmt.Errorf("unsupported protocol %q", cfg.Protocol)
	}
}

func normalizeGatewayTunnelConfig(cfg msg.GatewayTunnelConfig) (msg.GatewayTunnelConfig, error) {
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Remark = strings.TrimSpace(cfg.Remark)
	cfg.Protocol = strings.ToLower(strings.TrimSpace(cfg.Protocol))
	cfg.BindAddr = strings.TrimSpace(cfg.BindAddr)
	cfg.TargetHost = strings.TrimSpace(cfg.TargetHost)
	if cfg.TargetHost == "" {
		cfg.TargetHost = "127.0.0.1"
	}
	if cfg.ID == "" {
		return cfg, fmt.Errorf("gateway tunnel id is required")
	}
	if cfg.Name == "" {
		return cfg, fmt.Errorf("gateway tunnel name is required")
	}
	if cfg.Protocol != "tcp" && cfg.Protocol != "udp" {
		return cfg, fmt.Errorf("unsupported gateway tunnel protocol %q", cfg.Protocol)
	}
	if cfg.ListenPort <= 0 || cfg.ListenPort > 65535 {
		return cfg, fmt.Errorf("listenPort must be between 1 and 65535")
	}
	if cfg.TargetPort <= 0 || cfg.TargetPort > 65535 {
		return cfg, fmt.Errorf("targetPort must be between 1 and 65535")
	}
	if err := validateGatewayTunnelTargetHost(cfg.TargetHost); err != nil {
		return cfg, err
	}
	return cfg, nil
}

type gatewayTargetError struct {
	Status string
	Err    error
}

func (e *gatewayTargetError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func validateGatewayTunnelTargetReachability(cfg msg.GatewayTunnelConfig) error {
	addr := net.JoinHostPort(cfg.TargetHost, fmt.Sprintf("%d", cfg.TargetPort))
	switch cfg.Protocol {
	case "tcp":
		if _, err := net.ResolveTCPAddr("tcp", addr); err != nil {
			return &gatewayTargetError{Status: gatewaypkg.StatusTargetInvalid, Err: err}
		}
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return &gatewayTargetError{Status: gatewaypkg.StatusTargetUnreachable, Err: err}
		}
		_ = conn.Close()
	case "udp":
		if _, err := net.ResolveUDPAddr("udp", addr); err != nil {
			return &gatewayTargetError{Status: gatewaypkg.StatusTargetInvalid, Err: err}
		}
	}
	return nil
}

func validateGatewayTunnelTargetHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("targetHost is required")
	}
	if len(host) > 255 {
		return fmt.Errorf("targetHost is too long")
	}
	if strings.ContainsAny(host, "\r\n\t") {
		return fmt.Errorf("targetHost contains invalid whitespace")
	}
	return nil
}
