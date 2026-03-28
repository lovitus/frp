package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func TestLoadConfigureMixAliases(t *testing.T) {
	clientContent := []byte(`
serverAddr = "127.0.0.1"
mix_bind_port = 7000
mix_fallback_hosts = "10.20.0.65,kr.goodfood.com:7002"
mix_token = "kcp://kcppass,tcp://tcppass"
`)
	var clientCfg v1.ClientConfig
	require.NoError(t, LoadConfigure(clientContent, &clientCfg, true, "toml"))
	require.NoError(t, clientCfg.Complete())
	require.Equal(t, 7000, clientCfg.MixBindPort)
	require.Equal(t, "10.20.0.65,kr.goodfood.com:7002", clientCfg.MixFallbackHosts)
	require.Equal(t, "kcp://kcppass,tcp://tcppass", clientCfg.MixToken)

	serverContent := []byte(`
bindPort = 7001
mix_bind_port = 7000
mix_token = "kcp://kcppass,tcp://tcppass"
`)
	var serverCfg v1.ServerConfig
	require.NoError(t, LoadConfigure(serverContent, &serverCfg, true, "toml"))
	require.NoError(t, serverCfg.Complete())
	require.Equal(t, 7000, serverCfg.MixBindPort)
	require.Equal(t, "kcp://kcppass,tcp://tcppass", serverCfg.MixToken)
}

func TestLoadServerConfigLegacyMixAliases(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "frps.ini")
	content := []byte(`
[common]
bindPort = 7008
mix_bind_port = 7001
mix_token = "ss://chacha20-ietf-poly1305:test,kcp://test"
dashboard_port = 7501
`)
	require.NoError(t, os.WriteFile(configPath, content, 0o600))

	serverCfg, isLegacy, err := LoadServerConfig(configPath, true)
	require.NoError(t, err)
	require.True(t, isLegacy)
	require.Equal(t, 7008, serverCfg.BindPort)
	require.Equal(t, 7001, serverCfg.MixBindPort)
	require.Equal(t, "ss://chacha20-ietf-poly1305:test,kcp://test", serverCfg.MixToken)
	require.Equal(t, 7501, serverCfg.WebServer.Port)
}

func TestLoadClientConfigLegacyMixAliases(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "frpc.ini")
	content := []byte(`
[common]
serverAddr = "10.20.0.64"
mix_bind_port = 7001
mixToken = "kcp://test,ssh://user:test"
mix_fallback_hosts = "10.20.0.65,kr.example.com:7002"
clientID = "edge-kr-01"
mixAllowGateway = true
login_fail_exit = false
`)
	require.NoError(t, os.WriteFile(configPath, content, 0o600))

	result, err := LoadClientConfigResult(configPath, true)
	require.NoError(t, err)
	require.True(t, result.IsLegacyFormat)
	require.Equal(t, "10.20.0.64", result.Common.ServerAddr)
	require.Equal(t, 7001, result.Common.MixBindPort)
	require.Equal(t, "kcp://test,ssh://user:test", result.Common.MixToken)
	require.Equal(t, "10.20.0.65,kr.example.com:7002", result.Common.MixFallbackHosts)
	require.Equal(t, "edge-kr-01", result.Common.ClientID)
	require.NotNil(t, result.Common.AllowGatewayTunnels)
	require.True(t, *result.Common.AllowGatewayTunnels)
	require.NotNil(t, result.Common.LoginFailExit)
	require.False(t, *result.Common.LoginFailExit)
}
