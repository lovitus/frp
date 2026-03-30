package client

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fatedier/frp/pkg/config/source"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	gatewaypkg "github.com/fatedier/frp/pkg/gateway"
	"github.com/fatedier/frp/pkg/msg"
)

func TestBuildGatewayProxyConfigurerAddsGatewayAnnotations(t *testing.T) {
	t.Parallel()

	cfg, err := buildGatewayProxyConfigurer(gatewayProxyBuildInput{
		annotationCfg: msg.GatewayTunnelConfig{
			ID:         "tun-1",
			Name:       "db",
			Protocol:   "tcp",
			BindAddr:   "127.0.0.1",
			ListenPort: 6000,
			TargetHost: "127.0.0.1",
			TargetPort: 5432,
		},
		localHost: "127.0.0.1",
		localPort: 5432,
	})
	require.NoError(t, err)

	base := cfg.GetBaseConfig()
	require.Equal(t, gatewaypkg.ProxyName("tun-1"), base.Name)
	require.Equal(t, gatewaypkg.AnnotationSourceGatewayTunnel, base.Annotations[gatewaypkg.AnnotationSourceKey])
	require.Equal(t, "tun-1", base.Annotations[gatewaypkg.AnnotationTunnelIDKey])
	require.Equal(t, "db", base.Annotations[gatewaypkg.AnnotationTunnelNameKey])
	require.Equal(t, "", base.Annotations[gatewaypkg.AnnotationTunnelRemarkKey])
	require.Equal(t, "127.0.0.1", base.Annotations[gatewaypkg.AnnotationTargetHostKey])
	require.Equal(t, "5432", base.Annotations[gatewaypkg.AnnotationTargetPortKey])
	require.Equal(t, "127.0.0.1", base.Annotations[gatewaypkg.AnnotationBindAddrKey])
	require.Equal(t, gatewaypkg.TargetTypeDirect, base.Annotations[gatewaypkg.AnnotationTargetTypeKey])
	require.Equal(t, "127.0.0.1", base.LocalIP)
	require.Equal(t, 5432, base.LocalPort)
}

func TestBuildGatewayProxyConfigurerPreservesEmbeddedTargetAnnotations(t *testing.T) {
	t.Parallel()

	cfg, err := buildGatewayProxyConfigurer(gatewayProxyBuildInput{
		annotationCfg: msg.GatewayTunnelConfig{
			ID:         "tun-2",
			Name:       "ss",
			Protocol:   "tcp",
			BindAddr:   "0.0.0.0",
			ListenPort: 7000,
			TargetType: gatewaypkg.TargetTypeSSProxy,
			TargetHost: "gateway-proxy",
			TargetPort: 8388,
		},
		localHost: "127.0.0.1",
		localPort: 34567,
	})
	require.NoError(t, err)

	base := cfg.GetBaseConfig()
	require.Equal(t, gatewaypkg.TargetTypeSSProxy, base.Annotations[gatewaypkg.AnnotationTargetTypeKey])
	require.Equal(t, "gateway-proxy", base.Annotations[gatewaypkg.AnnotationTargetHostKey])
	require.Equal(t, "8388", base.Annotations[gatewaypkg.AnnotationTargetPortKey])
	require.Equal(t, "127.0.0.1", base.LocalIP)
	require.Equal(t, 34567, base.LocalPort)
}

func TestNormalizeGatewayTunnelConfigDefaultsTargetHost(t *testing.T) {
	t.Parallel()

	cfg, err := normalizeGatewayTunnelConfig(msg.GatewayTunnelConfig{
		ID:         "tun-2",
		Name:       "metrics",
		Protocol:   "udp",
		BindAddr:   "0.0.0.0",
		ListenPort: 9000,
		TargetPort: 9001,
	})
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", cfg.TargetHost)
}

func TestNormalizeGatewayTunnelConfigRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  msg.GatewayTunnelConfig
	}{
		{
			name: "missing name",
			cfg: msg.GatewayTunnelConfig{
				ID:         "tun-3",
				Protocol:   "tcp",
				BindAddr:   "0.0.0.0",
				ListenPort: 6000,
				TargetHost: "127.0.0.1",
				TargetPort: 22,
			},
		},
		{
			name: "unsupported protocol",
			cfg: msg.GatewayTunnelConfig{
				ID:         "tun-3",
				Name:       "bad",
				Protocol:   "http",
				BindAddr:   "0.0.0.0",
				ListenPort: 6000,
				TargetHost: "127.0.0.1",
				TargetPort: 22,
			},
		},
		{
			name: "invalid target host whitespace",
			cfg: msg.GatewayTunnelConfig{
				ID:         "tun-3",
				Name:       "bad",
				Protocol:   "tcp",
				BindAddr:   "0.0.0.0",
				ListenPort: 6000,
				TargetHost: "127.0.0.1\tbad",
				TargetPort: 22,
			},
		},
		{
			name: "invalid listen port",
			cfg: msg.GatewayTunnelConfig{
				ID:         "tun-3",
				Name:       "bad",
				Protocol:   "udp",
				BindAddr:   "0.0.0.0",
				ListenPort: 70000,
				TargetHost: "127.0.0.1",
				TargetPort: 53,
			},
		},
		{
			name: "ss proxy missing password",
			cfg: msg.GatewayTunnelConfig{
				ID:         "tun-4",
				Name:       "ss",
				Protocol:   "tcp",
				BindAddr:   "0.0.0.0",
				ListenPort: 6100,
				TargetType: gatewaypkg.TargetTypeSSProxy,
				SSMethod:   "chacha20-ietf-poly1305",
			},
		},
		{
			name: "socks5 udp unsupported",
			cfg: msg.GatewayTunnelConfig{
				ID:         "tun-5",
				Name:       "socks",
				Protocol:   "udp",
				BindAddr:   "0.0.0.0",
				ListenPort: 6101,
				TargetType: gatewaypkg.TargetTypeSocks5Proxy,
			},
		},
		{
			name: "ss udp method unsupported",
			cfg: msg.GatewayTunnelConfig{
				ID:         "tun-6",
				Name:       "ss-udp-bad",
				Protocol:   "udp",
				BindAddr:   "0.0.0.0",
				ListenPort: 6102,
				TargetType: gatewaypkg.TargetTypeSSProxy,
				SSMethod:   "none",
				SSPassword: "secret",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeGatewayTunnelConfig(tc.cfg)
			require.Error(t, err)
		})
	}
}

func TestNormalizeGatewayTunnelConfigAcceptsSSProxy(t *testing.T) {
	t.Parallel()

	cfg, err := normalizeGatewayTunnelConfig(msg.GatewayTunnelConfig{
		ID:         "tun-6",
		Name:       "ss",
		Protocol:   "udp",
		BindAddr:   "0.0.0.0",
		ListenPort: 6200,
		TargetType: gatewaypkg.TargetTypeSSProxy,
		SSMethod:   "chacha20-ietf-poly1305",
		SSPassword: "secret",
	})
	require.NoError(t, err)
	require.Equal(t, gatewaypkg.TargetTypeSSProxy, cfg.TargetType)
}

func TestApplyGatewayTunnelsRollsBackRuntimeSourceOnReloadFailure(t *testing.T) {
	t.Parallel()

	runtimeSource := source.NewConfigSource()
	oldCfg, err := buildGatewayProxyConfigurer(gatewayProxyBuildInput{
		annotationCfg: msg.GatewayTunnelConfig{
			ID:         "old-tun",
			Name:       "old",
			Protocol:   "tcp",
			BindAddr:   "127.0.0.1",
			ListenPort: 6000,
			TargetHost: "127.0.0.1",
			TargetPort: 22,
		},
		localHost: "127.0.0.1",
		localPort: 22,
	})
	require.NoError(t, err)
	require.NoError(t, runtimeSource.ReplaceAll([]v1.ProxyConfigurer{oldCfg}, nil))

	manager := NewGatewayTunnelManager(&Service{runtimeSource: runtimeSource}, runtimeSource, true)
	err = manager.ApplyGatewayTunnels([]msg.GatewayTunnelConfig{
		{
			ID:         "new-tun",
			Name:       "ss",
			Protocol:   "tcp",
			BindAddr:   "0.0.0.0",
			ListenPort: 7000,
			TargetType: gatewaypkg.TargetTypeSSProxy,
			SSMethod:   "chacha20-ietf-poly1305",
			SSPassword: "secret",
		},
	})
	require.Error(t, err)

	proxies, visitors, loadErr := runtimeSource.Load()
	require.NoError(t, loadErr)
	require.Empty(t, visitors)
	require.Len(t, proxies, 1)
	require.Equal(t, gatewaypkg.ProxyName("old-tun"), proxies[0].GetBaseConfig().Name)
}
