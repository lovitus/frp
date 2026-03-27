package config

import (
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
