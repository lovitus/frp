package gateway

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveBindAddr(t *testing.T) {
	t.Parallel()

	defaultBindAddr := "0.0.0.0"

	require.Equal(t, defaultBindAddr, ResolveBindAddr(nil, defaultBindAddr))
	require.Equal(t, defaultBindAddr, ResolveBindAddr(map[string]string{
		AnnotationBindAddrKey: "127.0.0.1",
	}, defaultBindAddr))
	require.Equal(t, "127.0.0.1", ResolveBindAddr(map[string]string{
		AnnotationSourceKey:   AnnotationSourceGatewayTunnel,
		AnnotationBindAddrKey: "127.0.0.1",
		AnnotationTunnelIDKey: "t-1",
	}, defaultBindAddr))
	require.Equal(t, "2401:c080:1c02:aaf:5400:8ff:fe88:d88f", ResolveBindAddr(map[string]string{
		AnnotationSourceKey:   AnnotationSourceGatewayTunnel,
		AnnotationBindAddrKey: "[2401:c080:1c02:aaf:5400:8ff:fe88:d88f]",
	}, defaultBindAddr))
	require.Equal(t, defaultBindAddr, ResolveBindAddr(map[string]string{
		AnnotationSourceKey:   AnnotationSourceGatewayTunnel,
		AnnotationBindAddrKey: "bad host",
	}, defaultBindAddr))
}
