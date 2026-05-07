package gateway

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateGatewaySingSSMethodAcceptsAEAD2022PSK(t *testing.T) {
	t.Parallel()

	psk128 := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	err := ValidateGatewaySingSSMethod("tcp", "2022-blake3-aes-128-gcm", psk128)
	require.NoError(t, err)

	psk256 := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	err = ValidateGatewaySingSSMethod("udp", "2022-blake3-aes-256-gcm", psk256)
	require.NoError(t, err)
}

func TestValidateGatewaySingSSMethodRejectsInvalidAEAD2022PSK(t *testing.T) {
	t.Parallel()

	err := ValidateGatewaySingSSMethod("tcp", "2022-blake3-aes-128-gcm", "plain-secret")
	require.Error(t, err)

	shortPSK := base64.StdEncoding.EncodeToString([]byte("short"))
	err = ValidateGatewaySingSSMethod("udp", "2022-blake3-aes-256-gcm", shortPSK)
	require.Error(t, err)
}
