package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func newTestMixManager(t *testing.T) *MixConnectorManager {
	t.Helper()
	manager, err := NewMixConnectorManager(&v1.ClientCommonConfig{
		MixToken: "kcp://kcppass,quic://quicpass,tcp://tcppass",
	})
	require.NoError(t, err)
	return manager
}

func TestMixManagerFallbackAfterThreeFailures(t *testing.T) {
	manager := newTestMixManager(t)

	failCount, fallback := manager.recordDialFailure(0)
	require.Equal(t, 1, failCount)
	require.Nil(t, fallback)

	failCount, fallback = manager.recordDialFailure(0)
	require.Equal(t, 2, failCount)
	require.Nil(t, fallback)

	failCount, fallback = manager.recordDialFailure(0)
	require.Equal(t, mixFallbackThreshold, failCount)
	require.NotNil(t, fallback)
	require.Equal(t, v1.MixProtocolQUIC, fallback.Protocol)
	require.Equal(t, 1, manager.CurrentActiveIndex())
}

func TestMixManagerSuccessResetsFailureCount(t *testing.T) {
	manager := newTestMixManager(t)

	manager.recordDialFailure(0)
	manager.recordDialFailure(0)
	manager.recordDialSuccess(0)

	failCount, fallback := manager.recordDialFailure(0)
	require.Equal(t, 1, failCount)
	require.Nil(t, fallback)
}

func TestMixManagerFallbackImmediatelyTargetsNextProtocol(t *testing.T) {
	manager := newTestMixManager(t)

	manager.recordDialFailure(0)
	manager.recordDialFailure(0)
	_, fallback := manager.recordDialFailure(0)
	require.NotNil(t, fallback)

	index, entry, waitFor := manager.prepareDial()
	require.Equal(t, 1, index)
	require.Equal(t, v1.MixProtocolQUIC, entry.Protocol)
	require.Zero(t, waitFor)
}

func TestMixManagerFallbackWrapsToFirstProtocol(t *testing.T) {
	manager := newTestMixManager(t)

	manager.mu.Lock()
	manager.activeIndex = 2
	manager.mu.Unlock()

	failCount, fallback := manager.recordDialFailure(2)
	require.Equal(t, 1, failCount)
	require.Nil(t, fallback)

	failCount, fallback = manager.recordDialFailure(2)
	require.Equal(t, 2, failCount)
	require.Nil(t, fallback)

	failCount, fallback = manager.recordDialFailure(2)
	require.Equal(t, mixFallbackThreshold, failCount)
	require.NotNil(t, fallback)
	require.Equal(t, v1.MixProtocolKCP, fallback.Protocol)
	require.Equal(t, 0, manager.CurrentActiveIndex())
}

func TestMixManagerFailbackCandidates(t *testing.T) {
	manager := newTestMixManager(t)
	_, fallback := manager.recordDialFailure(0)
	require.Nil(t, fallback)
	_, fallback = manager.recordDialFailure(0)
	require.Nil(t, fallback)
	_, fallback = manager.recordDialFailure(0)
	require.NotNil(t, fallback)
	manager.recordDialSuccess(1)

	manager.mu.Lock()
	manager.lastSwitchTime = time.Now().Add(-mixFailbackInterval)
	manager.mu.Unlock()

	baseIndex, candidates, ok := manager.nextFailbackCandidates()
	require.True(t, ok)
	require.Equal(t, 1, baseIndex)
	require.Len(t, candidates, 1)
	require.Equal(t, v1.MixProtocolKCP, candidates[0].Protocol.Protocol)
}

func TestMixManagerNoFailbackOnPrimary(t *testing.T) {
	manager := newTestMixManager(t)
	_, _, ok := manager.nextFailbackCandidates()
	require.False(t, ok)
}

func TestMixManagerSkipProbeWhileSwitching(t *testing.T) {
	manager := newTestMixManager(t)
	manager.recordDialFailure(0)
	manager.recordDialFailure(0)
	_, fallback := manager.recordDialFailure(0)
	require.NotNil(t, fallback)

	_, _, ok := manager.nextFailbackCandidates()
	require.False(t, ok)
}

func TestMixManagerStaleProbeCannotOverrideNewActive(t *testing.T) {
	manager := newTestMixManager(t)
	manager.mu.Lock()
	manager.activeIndex = 2
	manager.lastSwitchTime = time.Now().Add(-mixFailbackInterval)
	manager.mu.Unlock()

	baseIndex, _, ok := manager.nextFailbackCandidates()
	require.True(t, ok)

	manager.mu.Lock()
	manager.activeIndex = 1
	manager.probing = true
	manager.mu.Unlock()

	require.False(t, manager.switchToPreferred(baseIndex, 0))
}

func TestSetMixTimingForTestingRestoresDefaults(t *testing.T) {
	prevDelay := mixFallbackDelay
	prevInterval := mixFailbackInterval
	prevThreshold := mixFallbackThreshold

	restore := SetMixTimingForTesting(50*time.Millisecond, 75*time.Millisecond, 2)
	require.Equal(t, 50*time.Millisecond, mixFallbackDelay)
	require.Equal(t, 75*time.Millisecond, mixFailbackInterval)
	require.Equal(t, 2, mixFallbackThreshold)

	restore()
	require.Equal(t, prevDelay, mixFallbackDelay)
	require.Equal(t, prevInterval, mixFailbackInterval)
	require.Equal(t, prevThreshold, mixFallbackThreshold)
}

func TestBuildMixClientConfigSupportsSSH(t *testing.T) {
	cfg, err := buildMixClientConfig(&v1.ClientCommonConfig{MixBindPort: 7000}, v1.MixProtocolConfig{
		Protocol: v1.MixProtocolSSH,
	})
	require.NoError(t, err)
	require.Equal(t, v1.MixProtocolSSH, cfg.Transport.Protocol)
}
