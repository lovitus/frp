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
