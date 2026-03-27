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
