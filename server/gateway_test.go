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
	}, true, nil)
	require.NoError(t, err)
	require.NotEmpty(t, tunnel.ID)
	require.Equal(t, "0.0.0.0", tunnel.BindAddr)
	require.Equal(t, "127.0.0.1", tunnel.TargetHost)
	require.Equal(t, gatewaypkg.TargetTypeDirect, tunnel.TargetType)
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

func TestGatewayTunnelManagerUpdateUsesPathIDWhenPayloadIDIsEmpty(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		nil,
	)

	created, err := manager.Create(GatewayTunnel{
		Name:       "web",
		Protocol:   "tcp",
		BindAddr:   "127.0.0.1",
		ListenPort: 6000,
		ClientKey:  "client-a",
		TargetHost: "127.0.0.1",
		TargetPort: 8080,
	})
	require.NoError(t, err)

	updated, err := manager.Update(created.ID, GatewayTunnel{
		Name:       "web",
		Remark:     "updated",
		Protocol:   "tcp",
		BindAddr:   "127.0.0.1",
		ListenPort: 6000,
		ClientKey:  "client-a",
		TargetHost: "127.0.0.1",
		TargetPort: 8081,
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, updated.ID)
	require.Equal(t, "updated", updated.Remark)
	require.Equal(t, 8081, updated.TargetPort)
}

func TestGatewayTunnelManagerUpdateClearsInactiveTargetSecrets(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		nil,
	)

	created, err := manager.Create(GatewayTunnel{
		Name:          "proxy",
		Protocol:      "tcp",
		BindAddr:      "127.0.0.1",
		ListenPort:    6001,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitPermanent,
		ValidityValue: 0,
	})
	require.NoError(t, err)

	updated, err := manager.Update(created.ID, GatewayTunnel{
		Name:       "proxy",
		Protocol:   "tcp",
		BindAddr:   "127.0.0.1",
		ListenPort: 6001,
		ClientKey:  "client-a",
		TargetType: gatewaypkg.TargetTypeDirect,
		TargetHost: "127.0.0.1",
		TargetPort: 8080,
	})
	require.NoError(t, err)
	require.Equal(t, gatewaypkg.TargetTypeDirect, updated.TargetType)
	require.Empty(t, updated.SSMethod)
	require.Empty(t, updated.SSPassword)
	require.False(t, updated.Socks5Auth)
	require.Empty(t, updated.Socks5User)
	require.Empty(t, updated.Socks5Pass)
	require.Equal(t, "127.0.0.1", updated.TargetHost)
	require.Equal(t, 8080, updated.TargetPort)
}

func TestGatewayTunnelManagerUpdateDisablingSocks5AuthClearsStoredCredentials(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		nil,
	)

	created, err := manager.Create(GatewayTunnel{
		Name:          "proxy",
		Protocol:      "tcp",
		BindAddr:      "127.0.0.1",
		ListenPort:    6002,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSocks5Proxy,
		Socks5Auth:    true,
		Socks5User:    "demo",
		Socks5Pass:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitPermanent,
		ValidityValue: 0,
	})
	require.NoError(t, err)

	updated, err := manager.Update(created.ID, GatewayTunnel{
		Name:          "proxy",
		Protocol:      "tcp",
		BindAddr:      "127.0.0.1",
		ListenPort:    6002,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSocks5Proxy,
		Socks5Auth:    false,
		ValidityUnit:  gatewaypkg.ValidityUnitPermanent,
		ValidityValue: 0,
	})
	require.NoError(t, err)
	require.False(t, updated.Socks5Auth)
	require.Empty(t, updated.Socks5User)
	require.Empty(t, updated.Socks5Pass)
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
		{
			name: "socks5 udp unsupported",
			tunnel: GatewayTunnel{
				Name:       "web",
				Protocol:   "udp",
				ListenPort: 6000,
				ClientKey:  "client-a",
				TargetType: gatewaypkg.TargetTypeSocks5Proxy,
			},
		},
		{
			name: "ss udp method unsupported",
			tunnel: GatewayTunnel{
				Name:          "web",
				Protocol:      "udp",
				ListenPort:    6001,
				ClientKey:     "client-a",
				TargetType:    gatewaypkg.TargetTypeSSProxy,
				SSMethod:      "none",
				SSPassword:    "secret",
				ValidityUnit:  gatewaypkg.ValidityUnitPermanent,
				ValidityValue: 0,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeGatewayTunnel(tc.tunnel, true, nil)
			require.Error(t, err)
		})
	}
}

func TestGatewayTunnelManagerExpireDueFiltersWireConfig(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		nil,
	)
	created, err := manager.Create(GatewayTunnel{
		Name:          "temp-ss",
		Protocol:      "tcp",
		ListenPort:    6003,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 1,
	})
	require.NoError(t, err)

	manager.mu.Lock()
	manager.tunnels[created.ID].ExpiresAt = time.Now().Add(-time.Second)
	manager.mu.Unlock()

	affected := manager.ExpireDue(time.Now())
	require.Equal(t, []string{"client-a"}, affected)

	current, ok := manager.Get(created.ID)
	require.True(t, ok)
	require.Equal(t, gatewaypkg.StatusExpired, current.Status)

	wire := manager.listWireConfigsForClient("client-a")
	require.Empty(t, wire)
}

func TestGatewayTunnelManagerExpireDueStillSyncsAlreadyMarkedExpiredTunnel(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		nil,
	)
	created, err := manager.Create(GatewayTunnel{
		Name:          "temp-ss",
		Protocol:      "tcp",
		ListenPort:    6004,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 1,
	})
	require.NoError(t, err)

	now := time.Now()
	manager.mu.Lock()
	manager.tunnels[created.ID].ExpiresAt = now.Add(-time.Second)
	manager.tunnels[created.ID].Status = gatewaypkg.StatusExpired
	manager.tunnels[created.ID].Message = buildGatewayValidityStatus(manager.tunnels[created.ID])
	manager.mu.Unlock()

	affected := manager.ExpireDue(now)
	require.Empty(t, affected)
}

func TestNormalizeGatewayTunnelPreservesExpiryOnUpdateWhenValidityUnchanged(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(3 * time.Hour).UTC()
	tunnel, err := normalizeGatewayTunnel(GatewayTunnel{
		ID:            "tun-1",
		Name:          "temp",
		Protocol:      "tcp",
		ListenPort:    6005,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 6,
	}, false, &GatewayTunnel{
		ID:            "tun-1",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 6,
		ExpiresAt:     expiresAt,
	})
	require.NoError(t, err)
	require.True(t, tunnel.ExpiresAt.Equal(expiresAt))
}

func TestNormalizeGatewayTunnelRecomputesExpiredLeaseOnUpdate(t *testing.T) {
	t.Parallel()

	expiredAt := time.Now().Add(-time.Hour).UTC()
	tunnel, err := normalizeGatewayTunnel(GatewayTunnel{
		ID:            "tun-2",
		Name:          "temp",
		Protocol:      "tcp",
		ListenPort:    6013,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 6,
	}, false, &GatewayTunnel{
		ID:            "tun-2",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 6,
		ExpiresAt:     expiredAt,
	})
	require.NoError(t, err)
	require.True(t, tunnel.ExpiresAt.After(time.Now()))
	require.False(t, tunnel.ExpiresAt.Equal(expiredAt))
}

func TestNormalizeGatewayTunnelUsesProvidedExpiryForImportRestore(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(6 * time.Hour).UTC().Truncate(time.Second)
	tunnel, err := normalizeGatewayTunnel(GatewayTunnel{
		Name:          "temp",
		Protocol:      "tcp",
		ListenPort:    6006,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitDay,
		ValidityValue: 1,
		ExpiresAt:     expiresAt,
	}, true, nil)
	require.NoError(t, err)
	require.True(t, tunnel.ExpiresAt.Equal(expiresAt))
}

func TestGatewayTunnelManagerExpireDueReturnsAffectedOnlyOnce(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		nil,
	)
	created, err := manager.Create(GatewayTunnel{
		Name:          "temp-ss",
		Protocol:      "tcp",
		ListenPort:    6007,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 1,
	})
	require.NoError(t, err)

	now := time.Now()
	manager.mu.Lock()
	manager.tunnels[created.ID].ExpiresAt = now.Add(-time.Second)
	manager.mu.Unlock()

	require.Equal(t, []string{"client-a"}, manager.ExpireDue(now))
	require.Empty(t, manager.ExpireDue(now.Add(time.Second)))
}

func TestGatewayTunnelManagerRefreshStatusSyncsClientWhenTunnelExpires(t *testing.T) {
	t.Parallel()

	var syncCount int
	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) {
			return registry.ClientInfo{Key: "client-a", Online: true, AllowGatewayTunnels: true}, true
		},
		func(clientKey string, message msg.Message) error {
			require.Equal(t, "client-a", clientKey)
			if _, ok := message.(*msg.GatewayTunnelsSync); ok {
				syncCount++
			}
			return nil
		},
	)
	created, err := manager.Create(GatewayTunnel{
		Name:          "temp-ss",
		Protocol:      "tcp",
		ListenPort:    6008,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 1,
	})
	require.NoError(t, err)

	manager.mu.Lock()
	manager.tunnels[created.ID].ExpiresAt = time.Now().Add(-time.Second)
	manager.mu.Unlock()

	manager.RefreshStatus(context.Background(), []string{created.ID})
	require.Equal(t, 1, syncCount)
}

func TestGatewayTunnelManagerNextExpiryReturnsNearestFutureDeadline(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		nil,
	)
	_, err := manager.Create(GatewayTunnel{
		Name:          "temp-a",
		Protocol:      "tcp",
		ListenPort:    6009,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 1,
	})
	require.NoError(t, err)
	createdB, err := manager.Create(GatewayTunnel{
		Name:          "temp-b",
		Protocol:      "tcp",
		ListenPort:    6010,
		ClientKey:     "client-b",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 1,
	})
	require.NoError(t, err)

	now := time.Now()
	expected := now.Add(5 * time.Second).UTC().Truncate(time.Second)
	manager.mu.Lock()
	for _, tunnel := range manager.tunnels {
		tunnel.ExpiresAt = now.Add(10 * time.Second).UTC()
	}
	manager.tunnels[createdB.ID].ExpiresAt = expected
	manager.mu.Unlock()

	next, ok := manager.NextExpiry(now)
	require.True(t, ok)
	require.True(t, next.Equal(expected))
}

func TestGatewayTunnelManagerNextExpiryIgnoresAlreadyExpiredTunnels(t *testing.T) {
	t.Parallel()

	manager := NewGatewayTunnelManager(
		func(string) (registry.ClientInfo, bool) { return registry.ClientInfo{}, false },
		nil,
	)
	createdA, err := manager.Create(GatewayTunnel{
		Name:          "temp-a",
		Protocol:      "tcp",
		ListenPort:    6011,
		ClientKey:     "client-a",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 1,
	})
	require.NoError(t, err)
	createdB, err := manager.Create(GatewayTunnel{
		Name:          "temp-b",
		Protocol:      "tcp",
		ListenPort:    6012,
		ClientKey:     "client-b",
		TargetType:    gatewaypkg.TargetTypeSSProxy,
		SSMethod:      "chacha20-ietf-poly1305",
		SSPassword:    "secret",
		ValidityUnit:  gatewaypkg.ValidityUnitHour,
		ValidityValue: 1,
	})
	require.NoError(t, err)

	now := time.Now()
	expected := now.Add(8 * time.Second).UTC().Truncate(time.Second)
	manager.mu.Lock()
	manager.tunnels[createdA.ID].ExpiresAt = now.Add(-time.Second).UTC()
	manager.tunnels[createdB.ID].ExpiresAt = expected
	manager.mu.Unlock()

	next, ok := manager.NextExpiry(now)
	require.True(t, ok)
	require.True(t, next.Equal(expected))
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
