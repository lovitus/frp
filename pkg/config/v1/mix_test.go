package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMixToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		got, err := ParseMixToken("kcp://kcppass,quic://quicpass,ss://aes-256-gcm:sspass,wss://wsspass,ssh://user:sshpass,tcp://tcppass")
		require.NoError(t, err)
		require.Len(t, got, 6)
		require.Equal(t, MixProtocolKCP, got[0].Protocol)
		require.Equal(t, "aes-256-gcm", got[2].Method)
		require.Equal(t, "sspass", got[2].Password)
		require.Equal(t, "user", got[4].Username)
		require.Equal(t, "sshpass", got[4].Password)
	})

	t.Run("empty token", func(t *testing.T) {
		_, err := ParseMixToken("")
		require.Error(t, err)
	})

	t.Run("unknown scheme", func(t *testing.T) {
		_, err := ParseMixToken("udp://secret")
		require.Error(t, err)
	})

	t.Run("invalid ss", func(t *testing.T) {
		_, err := ParseMixToken("ss://missingpassword")
		require.Error(t, err)
	})

	t.Run("invalid ssh", func(t *testing.T) {
		_, err := ParseMixToken("ssh://useronly")
		require.Error(t, err)
	})

	t.Run("duplicate protocol", func(t *testing.T) {
		_, err := ParseMixToken("kcp://first,kcp://second")
		require.Error(t, err)
	})
}

func TestParseMixFallbackHosts(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		got, err := ParseMixFallbackHosts("10.20.0.65,kr.goodfood.com:7002,[2401:c080:1c02:aaf:5400:8ff:fe88:d88f]:7007", 7001)
		require.NoError(t, err)
		require.Len(t, got, 3)
		require.Equal(t, "10.20.0.65", got[0].Host)
		require.Equal(t, 7001, got[0].Port)
		require.Equal(t, "kr.goodfood.com", got[1].Host)
		require.Equal(t, 7002, got[1].Port)
		require.Equal(t, "2401:c080:1c02:aaf:5400:8ff:fe88:d88f", got[2].Host)
		require.Equal(t, 7007, got[2].Port)
	})

	t.Run("empty", func(t *testing.T) {
		got, err := ParseMixFallbackHosts("", 7001)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("invalid port", func(t *testing.T) {
		_, err := ParseMixFallbackHosts("kr.goodfood.com:notaport", 7001)
		require.Error(t, err)
	})

	t.Run("duplicate host", func(t *testing.T) {
		_, err := ParseMixFallbackHosts("10.20.0.65,10.20.0.65:7001", 7001)
		require.Error(t, err)
	})
}

func TestBuildMixEndpoints(t *testing.T) {
	got, err := BuildMixEndpoints("10.20.0.64", 7001, "10.20.0.65,kr.goodfood.com:7002")
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, "10.20.0.64:7001", got[0].Address())
	require.Equal(t, "10.20.0.65:7001", got[1].Address())
	require.Equal(t, "kr.goodfood.com:7002", got[2].Address())
}
