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
	sscore "github.com/shadowsocks/go-shadowsocks2/core"
)

type gatewayTunnelRuntime struct {
	config        msg.GatewayTunnelConfig
	proxyName     string
	validationErr string
	embedded      gatewayEmbeddedService
}

type GatewayTunnelManager struct {
	service       *Service
	runtimeSource *source.ConfigSource
	enabled       bool

	mu           sync.RWMutex
	tunnels      map[string]*gatewayTunnelRuntime
	lastApplyErr string
}

type gatewayProxyBuildInput struct {
	annotationCfg msg.GatewayTunnelConfig
	localHost     string
	localPort     int
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
	if m.runtimeSource == nil {
		return fmt.Errorf("runtime source is unavailable")
	}

	m.mu.RLock()
	previous := make(map[string]*gatewayTunnelRuntime, len(m.tunnels))
	for id, tunnel := range m.tunnels {
		previous[id] = tunnel
	}
	m.mu.RUnlock()
	prevLiveProxyCfgs, prevLiveVisitorCfgs := m.service.currentConfigurers()
	prevProxyCfgs, prevVisitors, err := m.runtimeSource.Load()
	if err != nil {
		return err
	}

	next := make(map[string]*gatewayTunnelRuntime, len(tunnels))
	reusedEmbedded := make(map[gatewayEmbeddedService]struct{})
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
		if cfg.TargetType != gatewaypkg.TargetTypeDirect {
			createdEmbedded := false
			// Common transport settings are fixed for the lifetime of the client;
			// config reloads only rebuild proxy and visitor configurers.
			if prev, ok := previous[cfg.ID]; ok && prev.embedded != nil && canReuseGatewayEmbeddedService(prev.config, cfg) {
				runtime.embedded = prev.embedded
				reusedEmbedded[runtime.embedded] = struct{}{}
			} else {
				maxUDPSessions := v1.DefaultMaxUDPSessions
				if m.service != nil && m.service.common != nil && m.service.common.Transport.MaxUDPSessions > 0 {
					maxUDPSessions = m.service.common.Transport.MaxUDPSessions
				}
				svc, err := newGatewayEmbeddedService(cfg, maxUDPSessions)
				if err != nil {
					runtime.validationErr = err.Error()
					next[cfg.ID] = runtime
					continue
				}
				runtime.embedded = svc
				createdEmbedded = true
			}
			host, port := runtime.embedded.Endpoint()
			proxyCfg, err := buildGatewayProxyConfigurer(gatewayProxyBuildInput{
				annotationCfg: cfg,
				localHost:     host,
				localPort:     port,
			})
			if err != nil {
				runtime.validationErr = err.Error()
				if createdEmbedded && runtime.embedded != nil {
					_ = runtime.embedded.Close()
					runtime.embedded = nil
				}
				next[cfg.ID] = runtime
				continue
			}
			next[cfg.ID] = runtime
			proxyCfgs = append(proxyCfgs, proxyCfg)
			continue
		}
		proxyCfg, err := buildGatewayProxyConfigurer(gatewayProxyBuildInput{
			annotationCfg: cfg,
			localHost:     cfg.TargetHost,
			localPort:     cfg.TargetPort,
		})
		if err != nil {
			runtime.validationErr = err.Error()
			next[cfg.ID] = runtime
			continue
		}
		next[cfg.ID] = runtime
		proxyCfgs = append(proxyCfgs, proxyCfg)
	}

	if err := m.runtimeSource.ReplaceAll(proxyCfgs, nil); err != nil {
		closeGatewayTunnelRuntimes(next, reusedEmbedded)
		return err
	}

	if err := m.service.reloadConfigFromSources(); err != nil {
		_ = m.runtimeSource.ReplaceAll(prevProxyCfgs, prevVisitors)
		_ = m.service.UpdateAllConfigurer(prevLiveProxyCfgs, prevLiveVisitorCfgs)
		closeGatewayTunnelRuntimes(next, reusedEmbedded)
		m.mu.Lock()
		m.lastApplyErr = err.Error()
		m.mu.Unlock()
		return err
	}

	m.mu.Lock()
	old := m.tunnels
	m.tunnels = next
	m.lastApplyErr = ""
	m.mu.Unlock()
	closeObsoleteGatewayTunnelRuntimes(old, next)
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
		if tunnel.config.TargetType == gatewaypkg.TargetTypeDirect {
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

func buildGatewayProxyConfigurer(input gatewayProxyBuildInput) (v1.ProxyConfigurer, error) {
	cfg := input.annotationCfg
	if cfg.TargetType == "" {
		cfg.TargetType = gatewaypkg.TargetTypeDirect
	}
	if input.localHost == "" {
		input.localHost = cfg.TargetHost
	}
	if input.localPort == 0 {
		input.localPort = cfg.TargetPort
	}
	base := v1.ProxyBaseConfig{
		Name: gatewaypkg.ProxyName(cfg.ID),
		Type: cfg.Protocol,
		Annotations: map[string]string{
			gatewaypkg.AnnotationSourceKey:       gatewaypkg.AnnotationSourceGatewayTunnel,
			gatewaypkg.AnnotationTunnelIDKey:     cfg.ID,
			gatewaypkg.AnnotationTunnelNameKey:   cfg.Name,
			gatewaypkg.AnnotationTunnelRemarkKey: cfg.Remark,
			gatewaypkg.AnnotationTargetTypeKey:   cfg.TargetType,
			gatewaypkg.AnnotationTargetHostKey:   cfg.TargetHost,
			gatewaypkg.AnnotationTargetPortKey:   fmt.Sprintf("%d", cfg.TargetPort),
			gatewaypkg.AnnotationBindAddrKey:     cfg.BindAddr,
		},
		ProxyBackend: v1.ProxyBackend{
			LocalIP:   input.localHost,
			LocalPort: input.localPort,
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
	cfg.TargetType = strings.ToLower(strings.TrimSpace(cfg.TargetType))
	cfg.TargetHost = strings.TrimSpace(cfg.TargetHost)
	cfg.SSMethod = strings.TrimSpace(cfg.SSMethod)
	cfg.SSPassword = strings.TrimSpace(cfg.SSPassword)
	cfg.Socks5User = strings.TrimSpace(cfg.Socks5User)
	cfg.Socks5Pass = strings.TrimSpace(cfg.Socks5Pass)
	if cfg.ID == "" {
		return cfg, fmt.Errorf("gateway tunnel id is required")
	}
	if cfg.Name == "" {
		return cfg, fmt.Errorf("gateway tunnel name is required")
	}
	if cfg.Protocol != "tcp" && cfg.Protocol != "udp" {
		return cfg, fmt.Errorf("unsupported gateway tunnel protocol %q", cfg.Protocol)
	}
	if cfg.TargetType == "" {
		cfg.TargetType = gatewaypkg.TargetTypeDirect
	}
	if cfg.ListenPort <= 0 || cfg.ListenPort > 65535 {
		return cfg, fmt.Errorf("listenPort must be between 1 and 65535")
	}
	switch cfg.TargetType {
	case gatewaypkg.TargetTypeDirect:
		cfg.SSMethod = ""
		cfg.SSPassword = ""
		cfg.UOTEnabled = false
		cfg.UOTVersion = 0
		cfg.Socks5Auth = false
		cfg.Socks5User = ""
		cfg.Socks5Pass = ""
		if cfg.TargetHost == "" {
			cfg.TargetHost = "127.0.0.1"
		}
		if cfg.TargetPort <= 0 || cfg.TargetPort > 65535 {
			return cfg, fmt.Errorf("targetPort must be between 1 and 65535")
		}
		if err := validateGatewayTunnelTargetHost(cfg.TargetHost); err != nil {
			return cfg, err
		}
	case gatewaypkg.TargetTypeSSProxy:
		cfg.TargetHost = ""
		cfg.TargetPort = 0
		cfg.UOTEnabled = false
		cfg.UOTVersion = 0
		cfg.Socks5Auth = false
		cfg.Socks5User = ""
		cfg.Socks5Pass = ""
		if cfg.SSMethod == "" {
			return cfg, fmt.Errorf("ssMethod is required for ss_proxy")
		}
		if cfg.SSPassword == "" {
			return cfg, fmt.Errorf("ssPassword is required for ss_proxy")
		}
		if err := validateGatewaySSMethod(cfg.Protocol, cfg.SSMethod, cfg.SSPassword); err != nil {
			return cfg, err
		}
	case gatewaypkg.TargetTypeSingSSProxy:
		cfg.TargetHost = ""
		cfg.TargetPort = 0
		cfg.Socks5Auth = false
		cfg.Socks5User = ""
		cfg.Socks5Pass = ""
		if cfg.SSMethod == "" {
			return cfg, fmt.Errorf("ssMethod is required for sing_ss_proxy")
		}
		if cfg.SSPassword == "" {
			return cfg, fmt.Errorf("ssPassword is required for sing_ss_proxy")
		}
		if cfg.Protocol == "udp" {
			cfg.UOTEnabled = false
			cfg.UOTVersion = 0
		} else if cfg.UOTEnabled {
			if cfg.UOTVersion == 0 {
				cfg.UOTVersion = 2
			}
			if cfg.UOTVersion != 1 && cfg.UOTVersion != 2 {
				return cfg, fmt.Errorf("uotVersion must be 1 or 2")
			}
		} else {
			cfg.UOTVersion = 0
		}
		if err := gatewaypkg.ValidateGatewaySingSSMethod(cfg.Protocol, cfg.SSMethod, cfg.SSPassword); err != nil {
			return cfg, err
		}
	case gatewaypkg.TargetTypeSocks5Proxy:
		cfg.TargetHost = ""
		cfg.TargetPort = 0
		cfg.SSMethod = ""
		cfg.SSPassword = ""
		cfg.UOTEnabled = false
		cfg.UOTVersion = 0
		if cfg.Protocol != "tcp" {
			return cfg, fmt.Errorf("socks5_proxy currently supports tcp only")
		}
		if !cfg.Socks5Auth {
			cfg.Socks5User = ""
			cfg.Socks5Pass = ""
		}
		if cfg.Socks5Auth && (cfg.Socks5User == "" || cfg.Socks5Pass == "") {
			return cfg, fmt.Errorf("socks5 username and password are required when auth is enabled")
		}
	default:
		return cfg, fmt.Errorf("unsupported targetType %q", cfg.TargetType)
	}
	return cfg, nil
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

func closeGatewayTunnelRuntimes(items map[string]*gatewayTunnelRuntime, keep map[gatewayEmbeddedService]struct{}) {
	for _, item := range items {
		if item != nil && item.embedded != nil {
			if _, ok := keep[item.embedded]; ok {
				continue
			}
			_ = item.embedded.Close()
		}
	}
}

func closeObsoleteGatewayTunnelRuntimes(oldItems, newItems map[string]*gatewayTunnelRuntime) {
	for id, item := range oldItems {
		if item == nil || item.embedded == nil {
			continue
		}
		if next, ok := newItems[id]; ok && next.embedded == item.embedded {
			continue
		}
		_ = item.embedded.Close()
	}
}
