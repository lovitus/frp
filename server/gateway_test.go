package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gatewaypkg "github.com/fatedier/frp/pkg/gateway"
	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/server/registry"
)

func TestNormalizeGatewayTunnelDefaults(t *testing.T) {
	t.Parallel()

	tunnel, err := normalizeGatewayTunnel(GatewayTunnel{
		Name:       "web",
		Protocol:   "tcp",
		ListenPort: 6000,
		ClientKey:  "client-a",
		TargetPort: 8080,
	}, true)
	require.NoError(t, err)
	require.NotEmpty(t, tunnel.ID)
	require.Equal(t, "0.0.0.0", tunnel.BindAddr)
	require.Equal(t, "127.0.0.1", tunnel.TargetHost)
}

func TestGatewayTunnelManagerSyncClientSendsWireConfig(t *testing.T) {
	t.Parallel()

	var sent *msg.GatewayTunnelsSync
	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		func(clientKey string, message msg.Message) error {
			require.Equal(t, "client-a", clientKey)
			syncMsg, ok := message.(*msg.GatewayTunnelsSync)
			require.True(t, ok)
			sent = syncMsg
			return nil
		},
	)

	created, err := manager.Create(GatewayTunnel{
		Name:       "web",
		Remark:     "runtime tunnel",
		Protocol:   "tcp",
		BindAddr:   "127.0.0.1",
		ListenPort: 6000,
		ClientKey:  "client-a",
		TargetHost: "127.0.0.1",
		TargetPort: 8080,
	})
	require.NoError(t, err)

	err = manager.SyncClient("client-a")
	require.NoError(t, err)
	require.NotNil(t, sent)
	require.Len(t, sent.Tunnels, 1)
	require.Equal(t, created.ID, sent.Tunnels[0].ID)
	require.Equal(t, "127.0.0.1", sent.Tunnels[0].BindAddr)
	require.Equal(t, 6000, sent.Tunnels[0].ListenPort)
}

func TestGatewayTunnelManagerRefreshStatusMarksOfflineAndDisabled(t *testing.T) {
	t.Parallel()

	clientInfo := map[string]registry.ClientInfo{
		"offline-client": {
			Key:                 "offline-client",
			AllowGatewayTunnels: true,
			Online:              false,
		},
		"disabled-client": {
			Key:                 "disabled-client",
			AllowGatewayTunnels: false,
			Online:              true,
		},
	}
	manager := NewGatewayTunnelManager(
		func(clientKey string) (registry.ClientInfo, bool) {
			info, ok := clientInfo[clientKey]
			return info, ok
		},
		func(string, msg.Message) error {
			t.Fatal("sendMessage should not be called for offline or disabled clients")
			return nil
		},
	)

	offlineTunnel, err := manager.Create(GatewayTunnel{
		Name:       "offline",
		Protocol:   "tcp",
		ListenPort: 6001,
		ClientKey:  "offline-client",
		TargetHost: "127.0.0.1",
		TargetPort: 8081,
	})
	require.NoError(t, err)
	disabledTunnel, err := manager.Create(GatewayTunnel{
		Name:       "disabled",
		Protocol:   "udp",
		ListenPort: 6002,
		ClientKey:  "disabled-client",
		TargetHost: "127.0.0.1",
		TargetPort: 8082,
	})
	require.NoError(t, err)

	manager.RefreshStatus(context.Background(), nil)

	offlineCurrent, ok := manager.Get(offlineTunnel.ID)
	require.True(t, ok)
	require.Equal(t, gatewaypkg.StatusClientOffline, offlineCurrent.Status)
	require.Equal(t, "client is offline", offlineCurrent.Message)

	disabledCurrent, ok := manager.Get(disabledTunnel.ID)
	require.True(t, ok)
	require.Equal(t, gatewaypkg.StatusDisabled, disabledCurrent.Status)
	require.Equal(t, "client does not allow gateway tunnels", disabledCurrent.Message)
}

func TestGatewayTunnelManagerHandleStatusResponseUpdatesTunnel(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		nil,
	)
	created, err := manager.Create(GatewayTunnel{
		Name:       "db",
		Protocol:   "tcp",
		ListenPort: 6100,
		ClientKey:  "client-a",
		TargetHost: "127.0.0.1",
		TargetPort: 5432,
	})
	require.NoError(t, err)

	manager.HandleStatusResponse("client-a", &msg.GatewayTunnelStatusResponse{
		RequestID: "req-1",
		Statuses: []msg.GatewayTunnelStatus{
			{
				ID:         created.ID,
				Name:       created.Name,
				Status:     gatewaypkg.StatusOnline,
				Message:    "",
				RemoteAddr: ":6100",
				UpdatedAt:  1700000000,
			},
		},
	})

	current, ok := manager.Get(created.ID)
	require.True(t, ok)
	require.Equal(t, gatewaypkg.StatusOnline, current.Status)
	require.Equal(t, ":6100", current.RemoteAddr)
	require.Equal(t, int64(1700000000), current.UpdatedAt.Unix())
}

func TestNormalizeGatewayTunnelRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tunnel GatewayTunnel
	}{
		{
			name: "invalid bind addr",
			tunnel: GatewayTunnel{
				Name:       "web",
				Protocol:   "tcp",
				BindAddr:   "bad-host",
				ListenPort: 6000,
				ClientKey:  "client-a",
				TargetHost: "127.0.0.1",
				TargetPort: 8080,
			},
		},
		{
			name: "invalid protocol",
			tunnel: GatewayTunnel{
				Name:       "web",
				Protocol:   "http",
				ListenPort: 6000,
				ClientKey:  "client-a",
				TargetHost: "127.0.0.1",
				TargetPort: 8080,
			},
		},
		{
			name: "invalid target host whitespace",
			tunnel: GatewayTunnel{
				Name:       "web",
				Protocol:   "tcp",
				ListenPort: 6000,
				ClientKey:  "client-a",
				TargetHost: "127.0.0.1\nx",
				TargetPort: 8080,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeGatewayTunnel(tc.tunnel, true)
			require.Error(t, err)
		})
	}
}

func TestGatewayTunnelManagerRejectsDuplicateNamePerClient(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		nil,
	)

	_, err := manager.Create(GatewayTunnel{
		Name:       "ssh",
		Protocol:   "tcp",
		ListenPort: 6000,
		ClientKey:  "client-a",
		TargetHost: "127.0.0.1",
		TargetPort: 22,
	})
	require.NoError(t, err)

	_, err = manager.Create(GatewayTunnel{
		Name:       "ssh",
		Protocol:   "udp",
		ListenPort: 6001,
		ClientKey:  "client-a",
		TargetHost: "127.0.0.1",
		TargetPort: 53,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestGatewayTunnelManagerRefreshStatusTimeoutMarksPending(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(clientKey string) (registry.ClientInfo, bool) {
			return registry.ClientInfo{
				Key:                 clientKey,
				Online:              true,
				AllowGatewayTunnels: true,
			}, true
		},
		func(string, msg.Message) error { return nil },
	)
	created, err := manager.Create(GatewayTunnel{
		Name:       "db",
		Protocol:   "tcp",
		ListenPort: 6100,
		ClientKey:  "client-a",
		TargetHost: "127.0.0.1",
		TargetPort: 5432,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	manager.RefreshStatus(ctx, []string{created.ID})

	current, ok := manager.Get(created.ID)
	require.True(t, ok)
	require.Equal(t, gatewaypkg.StatusPending, current.Status)
	require.Equal(t, "status request timed out", current.Message)
}
