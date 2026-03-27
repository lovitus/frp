package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func TestValidateClientMixConfigRequiresCoreFieldsWhenFallbackHostsSet(t *testing.T) {
	cfg := &v1.ClientCommonConfig{
		MixFallbackHosts: "10.20.0.65,kr.goodfood.com:7002",
	}

	err := validateClientMixConfig(cfg)
	require.Error(t, err)
	require.ErrorContains(t, err, "mixBindPort is required")
	require.ErrorContains(t, err, "mixToken is required")
}

func TestValidateClientMixConfigRejectsInvalidFallbackHosts(t *testing.T) {
	cfg := &v1.ClientCommonConfig{
		ServerAddr:       "10.20.0.64",
		MixBindPort:      7001,
		MixFallbackHosts: "kr.goodfood.com:notaport",
		MixToken:         "kcp://kcppass,ssh://user:sshpass",
	}

	err := validateClientMixConfig(cfg)
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid mix fallback host")
}
