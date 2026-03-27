package server

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	clientpkg "github.com/fatedier/frp/client"
	"github.com/fatedier/frp/pkg/config/source"
	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func getFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitForCondition(t *testing.T, fn func() bool) {
	waitForConditionWithin(t, 10*time.Second, fn)
}

func waitForConditionWithin(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func startMixServer(t *testing.T, token string) (*Service, context.CancelFunc, int) {
	t.Helper()
	mixPort, releaseMixPort := reserveDualStackPort(t)
	bindPort := getFreePort(t)

	serverCfg := &v1.ServerConfig{
		BindAddr:    "127.0.0.1",
		BindPort:    bindPort,
		MixBindPort: mixPort,
		MixToken:    token,
	}
	require.NoError(t, serverCfg.Complete())
	releaseMixPort()

	serverSvc, err := NewService(serverCfg)
	require.NoError(t, err)

	serverCtx, serverCancel := context.WithCancel(context.Background())
	go serverSvc.Run(serverCtx)
	t.Cleanup(func() {
		serverCancel()
		_ = serverSvc.Close()
	})
	return serverSvc, serverCancel, mixPort
}

func startMixServerOnPort(t *testing.T, token string, mixPort int) (*Service, context.CancelFunc) {
	t.Helper()

	bindPort := getFreePort(t)
	serverCfg := &v1.ServerConfig{
		BindAddr:    "127.0.0.1",
		BindPort:    bindPort,
		MixBindPort: mixPort,
		MixToken:    token,
	}
	require.NoError(t, serverCfg.Complete())

	serverSvc, err := NewService(serverCfg)
	require.NoError(t, err)

	serverCtx, serverCancel := context.WithCancel(context.Background())
	go serverSvc.Run(serverCtx)
	t.Cleanup(func() {
		serverCancel()
		_ = serverSvc.Close()
	})
	return serverSvc, serverCancel
}

func startMixClient(
	t *testing.T,
	mixPort int,
	token string,
	user string,
	loginFailExit bool,
) (*clientpkg.Service, context.CancelFunc) {
	t.Helper()
	clientCfg := &v1.ClientCommonConfig{
		ServerAddr:    "127.0.0.1",
		MixBindPort:   mixPort,
		MixToken:      token,
		User:          user,
		LoginFailExit: lo.ToPtr(loginFailExit),
	}
	clientCfg.Transport.DialServerTimeout = 2
	require.NoError(t, clientCfg.Complete())

	clientSvc, err := clientpkg.NewService(clientpkg.ServiceOptions{
		Common:                 clientCfg,
		ConfigSourceAggregator: source.NewAggregator(source.NewConfigSource()),
	})
	require.NoError(t, err)

	clientCtx, clientCancel := context.WithCancel(context.Background())
	go clientSvc.Run(clientCtx)

	t.Cleanup(func() {
		clientCancel()
		clientSvc.Close()
	})
	return clientSvc, clientCancel
}

func waitForOnlineProtocol(t *testing.T, serverSvc *Service, user string, protocol string, timeout time.Duration) {
	t.Helper()
	waitForConditionWithin(t, timeout, func() bool {
		items := serverSvc.clientRegistry.List()
		for _, item := range items {
			if item.User == user && item.Online && item.SelectedProtocol == protocol {
				return true
			}
		}
		return false
	})
}

func TestMixSingleProtocolLoginSuccess(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		protocol string
	}{
		{name: "tcp", token: "tcp://tcppass", protocol: "tcp"},
		{name: "kcp", token: "kcp://kcppass", protocol: "kcp"},
		{name: "quic", token: "quic://quicpass", protocol: "quic"},
		{name: "wss", token: "wss://wsspass", protocol: "wss"},
		{name: "ss", token: "ss://aes-256-gcm:sspass", protocol: "ss"},
		{name: "ssh", token: "ssh://user:sshpass", protocol: "ssh"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.protocol == "ss" {
				// Client and server share one test process here; disable the global
				// Shadowsocks salt replay filter so the in-process setup matches the
				// real frpc/frps deployment model where they run in separate processes.
				t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
			}
			mixPort, releaseMixPort := reserveDualStackPort(t)
			bindPort := getFreePort(t)

			serverCfg := &v1.ServerConfig{
				BindAddr:    "127.0.0.1",
				BindPort:    bindPort,
				MixBindPort: mixPort,
				MixToken:    tc.token,
			}
			require.NoError(t, serverCfg.Complete())
			releaseMixPort()

			serverSvc, err := NewService(serverCfg)
			require.NoError(t, err)
			serverCtx, serverCancel := context.WithCancel(context.Background())
			defer serverCancel()
			serverDone := make(chan struct{})
			go func() {
				serverSvc.Run(serverCtx)
				close(serverDone)
			}()

			clientCfg := &v1.ClientCommonConfig{
				ServerAddr:    "127.0.0.1",
				MixBindPort:   mixPort,
				MixToken:      tc.token,
				LoginFailExit: lo.ToPtr(true),
			}
			require.NoError(t, clientCfg.Complete())
			agg := source.NewAggregator(source.NewConfigSource())
			clientSvc, err := clientpkg.NewService(clientpkg.ServiceOptions{
				Common:                 clientCfg,
				ConfigSourceAggregator: agg,
			})
			require.NoError(t, err)

			clientCtx, clientCancel := context.WithCancel(context.Background())
			defer clientCancel()
			clientDone := make(chan error, 1)
			go func() {
				clientDone <- clientSvc.Run(clientCtx)
			}()

			waitForCondition(t, func() bool {
				items := serverSvc.clientRegistry.List()
				if len(items) != 1 {
					return false
				}
				return items[0].SelectedProtocol == tc.protocol && items[0].Online
			})

			require.Equal(t, tc.protocol, clientSvc.StatusExporter().SelectedProtocol())

			clientCancel()
			clientSvc.Close()
			serverCancel()
			_ = serverSvc.Close()
			select {
			case <-serverDone:
			case <-time.After(500 * time.Millisecond):
			}
			select {
			case <-clientDone:
			case <-time.After(500 * time.Millisecond):
			}
		})
	}
}

func TestMixFallbackToNextProtocol(t *testing.T) {
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
	restoreMixTiming := clientpkg.SetMixTimingForTesting(100*time.Millisecond, 200*time.Millisecond, 3)
	defer restoreMixTiming()

	mixPort, releaseMixPort := reserveDualStackPort(t)
	bindPort := getFreePort(t)
	serverToken := "ss://aes-256-gcm:sspass"
	clientToken := "tcp://wrong,ss://aes-256-gcm:sspass"

	serverCfg := &v1.ServerConfig{
		BindAddr:    "127.0.0.1",
		BindPort:    bindPort,
		MixBindPort: mixPort,
		MixToken:    serverToken,
	}
	require.NoError(t, serverCfg.Complete())
	releaseMixPort()
	serverSvc, err := NewService(serverCfg)
	require.NoError(t, err)
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	go serverSvc.Run(serverCtx)

	clientCfg := &v1.ClientCommonConfig{
		ServerAddr:    "127.0.0.1",
		MixBindPort:   mixPort,
		MixToken:      clientToken,
		LoginFailExit: lo.ToPtr(false),
	}
	require.NoError(t, clientCfg.Complete())
	clientSvc, err := clientpkg.NewService(clientpkg.ServiceOptions{
		Common:                 clientCfg,
		ConfigSourceAggregator: source.NewAggregator(source.NewConfigSource()),
	})
	require.NoError(t, err)

	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()
	go clientSvc.Run(clientCtx)

	waitForCondition(t, func() bool {
		items := serverSvc.clientRegistry.List()
		return len(items) == 1 && items[0].SelectedProtocol == "ss"
	})

	require.Equal(t, "ss", clientSvc.StatusExporter().SelectedProtocol())
	clientSvc.Close()
	serverCancel()
	_ = serverSvc.Close()
}

func TestMixFallbackToThirdProtocol(t *testing.T) {
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
	restoreMixTiming := clientpkg.SetMixTimingForTesting(100*time.Millisecond, 200*time.Millisecond, 3)
	defer restoreMixTiming()

	serverToken := "kcp://kcppass,quic://quicpass,ss://aes-256-gcm:sspass"
	serverSvc, _, mixPort := startMixServer(t, serverToken)
	require.NotNil(t, serverSvc.mixKCPListener)
	require.NotNil(t, serverSvc.mixQUICListener)
	_ = serverSvc.mixKCPListener.Close()
	_ = serverSvc.mixQUICListener.Close()

	clientSvc, _ := startMixClient(t, mixPort, serverToken, "fallback-third", false)
	waitForOnlineProtocol(t, serverSvc, "fallback-third", "ss", 25*time.Second)
	require.Equal(t, "ss", clientSvc.StatusExporter().SelectedProtocol())
}

func TestMixFailbackToPreferredProtocol(t *testing.T) {
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
	restoreMixTiming := clientpkg.SetMixTimingForTesting(100*time.Millisecond, 200*time.Millisecond, 3)
	defer restoreMixTiming()

	serverToken := "tcp://tcppass,ss://aes-256-gcm:sspass"
	serverSvc, _, mixPort := startMixServer(t, serverToken)
	serverSvc.mixConfig.protocols[v1.MixProtocolTCP] = v1.MixProtocolConfig{
		Protocol: v1.MixProtocolTCP,
		Password: "wrong-tcp",
	}

	clientSvc, _ := startMixClient(t, mixPort, serverToken, "failback-primary", false)
	waitForOnlineProtocol(t, serverSvc, "failback-primary", "ss", 20*time.Second)
	require.Equal(t, "ss", clientSvc.StatusExporter().SelectedProtocol())

	serverSvc.mixConfig.protocols[v1.MixProtocolTCP] = v1.MixProtocolConfig{
		Protocol: v1.MixProtocolTCP,
		Password: "tcppass",
	}

	waitForOnlineProtocol(t, serverSvc, "failback-primary", "tcp", 20*time.Second)
	require.Equal(t, "tcp", clientSvc.StatusExporter().SelectedProtocol())
}

func TestMixHostFallbackAndFailbackToPrimaryHost(t *testing.T) {
	restoreMixTiming := clientpkg.SetMixTimingForTesting(100*time.Millisecond, 200*time.Millisecond, 3)
	defer restoreMixTiming()

	primaryMixPort, releasePrimaryMixPort := reserveDualStackPort(t)
	releasePrimaryMixPort()

	serverToken := "tcp://tcppass"
	backupSvc, _, backupMixPort := startMixServer(t, serverToken)

	clientCfg := &v1.ClientCommonConfig{
		ServerAddr:       "127.0.0.1",
		MixBindPort:      primaryMixPort,
		MixFallbackHosts: fmt.Sprintf("127.0.0.1:%d", backupMixPort),
		MixToken:         serverToken,
		User:             "host-failback",
		LoginFailExit:    lo.ToPtr(false),
	}
	clientCfg.Transport.DialServerTimeout = 2
	require.NoError(t, clientCfg.Complete())

	clientSvc, err := clientpkg.NewService(clientpkg.ServiceOptions{
		Common:                 clientCfg,
		ConfigSourceAggregator: source.NewAggregator(source.NewConfigSource()),
	})
	require.NoError(t, err)

	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()
	go clientSvc.Run(clientCtx)

	waitForOnlineProtocol(t, backupSvc, "host-failback", "tcp", 20*time.Second)
	require.Equal(t, "tcp", clientSvc.StatusExporter().SelectedProtocol())

	primarySvc, _ := startMixServerOnPort(t, serverToken, primaryMixPort)
	waitForOnlineProtocol(t, primarySvc, "host-failback", "tcp", 20*time.Second)
	require.Equal(t, "tcp", clientSvc.StatusExporter().SelectedProtocol())
}

func TestMixSharedPortProtocols(t *testing.T) {
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")

	serverToken := "kcp://kcppass,quic://quicpass,ss://aes-256-gcm:sspass,wss://wsspass,ssh://user:sshpass,tcp://tcppass"
	serverSvc, _, mixPort := startMixServer(t, serverToken)

	tests := []struct {
		name     string
		token    string
		protocol string
	}{
		{name: "kcp", token: "kcp://kcppass", protocol: "kcp"},
		{name: "quic", token: "quic://quicpass", protocol: "quic"},
		{name: "ss", token: "ss://aes-256-gcm:sspass", protocol: "ss"},
		{name: "wss", token: "wss://wsspass", protocol: "wss"},
		{name: "ssh", token: "ssh://user:sshpass", protocol: "ssh"},
		{name: "tcp", token: "tcp://tcppass", protocol: "tcp"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			user := "shared-" + tc.name
			clientSvc, clientCancel := startMixClient(t, mixPort, tc.token, user, true)
			waitForOnlineProtocol(t, serverSvc, user, tc.protocol, 10*time.Second)
			require.Equal(t, tc.protocol, clientSvc.StatusExporter().SelectedProtocol())

			clientCancel()
			clientSvc.Close()
		})
	}
}

func TestMixRejectsUnknownTCPTrafficQuickly(t *testing.T) {
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")

	_, _, mixPort := startMixServer(t, "ss://aes-256-gcm:sspass,ssh://user:sshpass,wss://wsspass")

	rawConn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(mixPort)), time.Second)
	require.NoError(t, err)
	defer rawConn.Close()

	_, err = rawConn.Write([]byte{0x01})
	require.NoError(t, err)
	_ = rawConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1)
	_, err = rawConn.Read(buf)
	require.Error(t, err)
}
