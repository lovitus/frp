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
		ServerAddr:  "primary.example.com",
		MixBindPort: 7000,
		MixToken:    "kcp://kcppass,quic://quicpass,tcp://tcppass",
	})
	require.NoError(t, err)
	return manager
}

func newHostFallbackMixManager(t *testing.T) *MixConnectorManager {
	t.Helper()
	manager, err := NewMixConnectorManager(&v1.ClientCommonConfig{
		ServerAddr:       "10.20.0.64",
		MixBindPort:      7001,
		MixFallbackHosts: "10.20.0.65,kr.goodfood.com:7002",
		MixToken:         "kcp://kcppass,ss://aes-256-gcm:sspass",
	})
	require.NoError(t, err)
	return manager
}

func newTwoHostThreeProtocolMixManager(t *testing.T) *MixConnectorManager {
	t.Helper()
	manager, err := NewMixConnectorManager(&v1.ClientCommonConfig{
		ServerAddr:       "10.20.0.64",
		MixBindPort:      7001,
		MixFallbackHosts: "10.20.0.65",
		MixToken:         "kcp://kcppass,ss://aes-256-gcm:sspass,ssh://frp:sshpass",
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
	require.Equal(t, v1.MixProtocolQUIC, fallback.Protocol.Protocol)
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
	require.Equal(t, v1.MixProtocolQUIC, entry.Protocol.Protocol)
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
	require.Equal(t, v1.MixProtocolKCP, fallback.Protocol.Protocol)
	require.Equal(t, 0, manager.CurrentActiveIndex())
}

func TestMixManagerFallbackWrapsAcrossHostsToFirstCandidate(t *testing.T) {
	manager := newHostFallbackMixManager(t)

	manager.mu.Lock()
	manager.activeIndex = len(manager.candidates) - 1
	manager.mu.Unlock()

	failCount, fallback := manager.recordDialFailure(len(manager.candidates) - 1)
	require.Equal(t, 1, failCount)
	require.Nil(t, fallback)

	failCount, fallback = manager.recordDialFailure(len(manager.candidates) - 1)
	require.Equal(t, 2, failCount)
	require.Nil(t, fallback)

	failCount, fallback = manager.recordDialFailure(len(manager.candidates) - 1)
	require.Equal(t, mixFallbackThreshold, failCount)
	require.NotNil(t, fallback)
	require.Equal(t, "10.20.0.64:7001", fallback.Address())
	require.Equal(t, v1.MixProtocolKCP, fallback.Protocol.Protocol)
	require.Equal(t, 0, manager.CurrentActiveIndex())
}

func TestMixManagerActiveCandidateTransientFailureAndRecovery(t *testing.T) {
	manager := newTwoHostThreeProtocolMixManager(t)
	require.Len(t, manager.candidates, 6)

	manager.mu.Lock()
	manager.activeIndex = 3 // second host + first protocol.
	manager.mu.Unlock()

	failCount, fallback := manager.recordDialFailure(3)
	require.Equal(t, 1, failCount)
	require.Nil(t, fallback)
	require.Equal(t, 3, manager.CurrentActiveIndex())

	failCount, fallback = manager.recordDialFailure(3)
	require.Equal(t, 2, failCount)
	require.Nil(t, fallback)
	require.Equal(t, 3, manager.CurrentActiveIndex())

	manager.recordDialSuccess(3)
	failCount, fallback = manager.recordDialFailure(3)
	require.Equal(t, 1, failCount)
	require.Nil(t, fallback)
	require.Equal(t, 3, manager.CurrentActiveIndex())
}

func TestMixManagerFallbackFromFourthCandidateMovesToFifth(t *testing.T) {
	manager := newTwoHostThreeProtocolMixManager(t)
	require.Len(t, manager.candidates, 6)

	manager.mu.Lock()
	manager.activeIndex = 3 // second host + first protocol.
	manager.mu.Unlock()

	failCount, fallback := manager.recordDialFailure(3)
	require.Equal(t, 1, failCount)
	require.Nil(t, fallback)
	failCount, fallback = manager.recordDialFailure(3)
	require.Equal(t, 2, failCount)
	require.Nil(t, fallback)
	failCount, fallback = manager.recordDialFailure(3)
	require.Equal(t, mixFallbackThreshold, failCount)
	require.NotNil(t, fallback)
	require.Equal(t, 4, manager.CurrentActiveIndex())
	require.Equal(t, manager.candidates[4].Address(), fallback.Address())
	require.Equal(t, manager.candidates[4].Protocol.Protocol, fallback.Protocol.Protocol)
}

func TestMixManagerStaleFailureDoesNotUseCurrentFailCount(t *testing.T) {
	manager := newTestMixManager(t)

	failCount, fallback := manager.recordDialFailure(0)
	require.Equal(t, 1, failCount)
	require.Nil(t, fallback)

	manager.mu.Lock()
	manager.activeIndex = 1
	manager.mu.Unlock()

	failCount, fallback = manager.recordDialFailure(0)
	require.Equal(t, 0, failCount)
	require.Nil(t, fallback)
}

func TestMixManagerBuildsHostAndProtocolCandidateOrder(t *testing.T) {
	manager := newHostFallbackMixManager(t)

	require.Len(t, manager.candidates, 6)
	require.Equal(t, "10.20.0.64:7001", manager.candidates[0].Address())
	require.Equal(t, v1.MixProtocolKCP, manager.candidates[0].Protocol.Protocol)
	require.Equal(t, "10.20.0.64:7001", manager.candidates[1].Address())
	require.Equal(t, v1.MixProtocolSS, manager.candidates[1].Protocol.Protocol)
	require.Equal(t, "10.20.0.65:7001", manager.candidates[2].Address())
	require.Equal(t, v1.MixProtocolKCP, manager.candidates[2].Protocol.Protocol)
	require.Equal(t, "kr.goodfood.com:7002", manager.candidates[5].Address())
	require.Equal(t, v1.MixProtocolSS, manager.candidates[5].Protocol.Protocol)
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
	require.Equal(t, v1.MixProtocolKCP, candidates[0].Candidate.Protocol.Protocol)
}

func TestMixManagerFailbackNeedsConsecutiveProbeSuccesses(t *testing.T) {
	manager := newTestMixManager(t)
	manager.mu.Lock()
	manager.activeIndex = 1
	manager.lastSwitchTime = time.Now().Add(-mixFailbackInterval)
	manager.mu.Unlock()

	baseIndex, candidates, ok := manager.nextFailbackCandidates()
	require.True(t, ok)
	require.Len(t, candidates, 1)
	count, ready := manager.recordFailbackProbeSuccess(baseIndex, candidates[0].Index)
	require.Equal(t, 1, count)
	require.False(t, ready)
	manager.finishProbe()

	baseIndex, candidates, ok = manager.nextFailbackCandidates()
	require.True(t, ok)
	count, ready = manager.recordFailbackProbeSuccess(baseIndex, candidates[0].Index)
	require.Equal(t, 2, count)
	require.False(t, ready)
	manager.finishProbe()

	baseIndex, candidates, ok = manager.nextFailbackCandidates()
	require.True(t, ok)
	count, ready = manager.recordFailbackProbeSuccess(baseIndex, candidates[0].Index)
	require.Equal(t, mixFailbackThreshold, count)
	require.True(t, ready)
}

func TestMixManagerFailbackProbeFailureResetsSuccessCount(t *testing.T) {
	manager := newTestMixManager(t)
	manager.mu.Lock()
	manager.activeIndex = 1
	manager.lastSwitchTime = time.Now().Add(-mixFailbackInterval)
	manager.mu.Unlock()

	baseIndex, candidates, ok := manager.nextFailbackCandidates()
	require.True(t, ok)
	count, ready := manager.recordFailbackProbeSuccess(baseIndex, candidates[0].Index)
	require.Equal(t, 1, count)
	require.False(t, ready)
	manager.finishProbe()

	baseIndex, _, ok = manager.nextFailbackCandidates()
	require.True(t, ok)
	manager.recordFailbackProbeFailure(baseIndex, 0)
	manager.finishProbe()

	baseIndex, candidates, ok = manager.nextFailbackCandidates()
	require.True(t, ok)
	count, ready = manager.recordFailbackProbeSuccess(baseIndex, candidates[0].Index)
	require.Equal(t, 1, count)
	require.False(t, ready)
}

func TestMixManagerFailbackProbeFailureOnOtherCandidateDoesNotResetStreak(t *testing.T) {
	manager := newTestMixManager(t)
	manager.mu.Lock()
	manager.activeIndex = 2
	manager.lastSwitchTime = time.Now().Add(-mixFailbackInterval)
	manager.mu.Unlock()

	for i := 1; i <= mixFailbackThreshold; i++ {
		baseIndex, candidates, ok := manager.nextFailbackCandidates()
		require.True(t, ok)
		require.Len(t, candidates, 2)
		require.Equal(t, 0, candidates[0].Index)
		require.Equal(t, 1, candidates[1].Index)

		// Candidate 0 keeps failing, but should not reset candidate 1 streak.
		manager.recordFailbackProbeFailure(baseIndex, candidates[0].Index)

		count, ready := manager.recordFailbackProbeSuccess(baseIndex, candidates[1].Index)
		require.Equal(t, i, count)
		if i < mixFailbackThreshold {
			require.False(t, ready)
			manager.finishProbe()
		} else {
			require.True(t, ready)
		}
	}
}

func TestMixManagerFailbackProbeCountResetsWhenActiveChanges(t *testing.T) {
	manager := newTestMixManager(t)
	manager.mu.Lock()
	manager.activeIndex = 1
	manager.lastSwitchTime = time.Now().Add(-mixFailbackInterval)
	manager.mu.Unlock()

	baseIndex, candidates, ok := manager.nextFailbackCandidates()
	require.True(t, ok)
	count, ready := manager.recordFailbackProbeSuccess(baseIndex, candidates[0].Index)
	require.Equal(t, 1, count)
	require.False(t, ready)
	manager.finishProbe()

	manager.recordDialSuccess(2)

	manager.mu.Lock()
	manager.lastSwitchTime = time.Now().Add(-mixFailbackInterval)
	manager.mu.Unlock()

	baseIndex, candidates, ok = manager.nextFailbackCandidates()
	require.True(t, ok)
	count, ready = manager.recordFailbackProbeSuccess(baseIndex, candidates[0].Index)
	require.Equal(t, 1, count)
	require.False(t, ready)
}

func TestMixManagerFailbackCandidatesCoverEarlierHostsAndProtocols(t *testing.T) {
	manager := newHostFallbackMixManager(t)
	manager.mu.Lock()
	manager.activeIndex = 5
	manager.lastSwitchTime = time.Now().Add(-mixFailbackInterval)
	manager.mu.Unlock()

	baseIndex, candidates, ok := manager.nextFailbackCandidates()
	require.True(t, ok)
	require.Equal(t, 5, baseIndex)
	require.Len(t, candidates, 5)
	require.Equal(t, "10.20.0.64:7001", candidates[0].Candidate.Address())
	require.Equal(t, v1.MixProtocolKCP, candidates[0].Candidate.Protocol.Protocol)
	require.Equal(t, "10.20.0.64:7001", candidates[1].Candidate.Address())
	require.Equal(t, v1.MixProtocolSS, candidates[1].Candidate.Protocol.Protocol)
	require.Equal(t, "10.20.0.65:7001", candidates[2].Candidate.Address())
	require.Equal(t, v1.MixProtocolKCP, candidates[2].Candidate.Protocol.Protocol)
	require.Equal(t, "10.20.0.65:7001", candidates[3].Candidate.Address())
	require.Equal(t, v1.MixProtocolSS, candidates[3].Candidate.Protocol.Protocol)
	require.Equal(t, "kr.goodfood.com:7002", candidates[4].Candidate.Address())
	require.Equal(t, v1.MixProtocolKCP, candidates[4].Candidate.Protocol.Protocol)
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
	cfg, err := buildMixClientConfig(&v1.ClientCommonConfig{
		ServerAddr:  "primary.example.com",
		MixBindPort: 7000,
	}, mixDialCandidate{
		Endpoint: v1.MixEndpointConfig{
			Host: "backup.example.com",
			Port: 7002,
		},
		Protocol: v1.MixProtocolConfig{
			Protocol: v1.MixProtocolSSH,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "backup.example.com", cfg.ServerAddr)
	require.Equal(t, 7002, cfg.MixBindPort)
	require.Equal(t, v1.MixProtocolSSH, cfg.Transport.Protocol)
}
