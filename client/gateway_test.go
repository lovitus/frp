package client

import (
	"testing"

	"github.com/stretchr/testify/require"

	gatewaypkg "github.com/fatedier/frp/pkg/gateway"
	"github.com/fatedier/frp/pkg/msg"
)

func TestBuildGatewayProxyConfigurerAddsGatewayAnnotations(t *testing.T) {
	t.Parallel()

	cfg, err := buildGatewayProxyConfigurer(msg.GatewayTunnelConfig{
		ID:         "tun-1",
		Name:       "db",
		Protocol:   "tcp",
		BindAddr:   "127.0.0.1",
		ListenPort: 6000,
		TargetHost: "127.0.0.1",
		TargetPort: 5432,
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
	require.Equal(t, "127.0.0.1", base.LocalIP)
	require.Equal(t, 5432, base.LocalPort)
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeGatewayTunnelConfig(tc.cfg)
			require.Error(t, err)
		})
	}
}
