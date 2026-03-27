package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/samber/lo"

	clientpkg "github.com/fatedier/frp/client"
	"github.com/fatedier/frp/pkg/config/source"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/util/log"
)

type benchServerRunner struct {
	svc      *Service
	cancel   context.CancelFunc
	done     chan struct{}
	mixPort  int
	bindPort int
}

type benchClientRunner struct {
	key     string
	svc     *clientpkg.Service
	cancel  context.CancelFunc
	started time.Time
}

type mixBenchResult struct {
	Scenario           string  `json:"scenario"`
	Protocol           string  `json:"protocol"`
	Concurrency        int     `json:"concurrency"`
	Iterations         int     `json:"iterations"`
	Attempts           int     `json:"attempts"`
	Successes          int     `json:"successes"`
	SuccessRate        float64 `json:"successRate"`
	AvgConnectMs       float64 `json:"avgConnectMs,omitempty"`
	P95ConnectMs       float64 `json:"p95ConnectMs,omitempty"`
	FallbackLatencyMs  float64 `json:"fallbackLatencyMs,omitempty"`
	FailbackLatencyMs  float64 `json:"failbackLatencyMs,omitempty"`
	HoldSeconds        float64 `json:"holdSeconds,omitempty"`
	ApproxProbeCount   int     `json:"approxProbeCount,omitempty"`
	LogBytes           int64   `json:"logBytes,omitempty"`
	Notes              string  `json:"notes,omitempty"`
	PeakOnlineObserved int     `json:"peakOnlineObserved,omitempty"`
}

func TestMixBench(t *testing.T) {
	if os.Getenv("FRP_RUN_MIX_BENCH") != "1" {
		t.Skip("set FRP_RUN_MIX_BENCH=1 to run mix benchmark scenarios")
	}

	outputDir := os.Getenv("FRP_MIX_BENCH_DIR")
	if outputDir == "" {
		outputDir = filepath.Join("tmp", "mix-bench")
	}
	requireNoError(t, os.MkdirAll(outputDir, 0o755))

	logFile := filepath.Join(outputDir, "mix-bench.log")
	log.InitLogger(logFile, "info", 3, true)

	restoreTiming := clientpkg.SetMixTimingForTesting(100*time.Millisecond, 200*time.Millisecond, 3)
	defer restoreTiming()

	var results []mixBenchResult

	results = append(results, runSingleProtocolStableBench(t)...)
	results = append(results, runSingleProtocolBurstBench(t)...)
	results = append(results, runMixStableScaleBench(t)...)
	results = append(results, runMixSoakBench(t)...)
	results = append(results, runMixIntermittentFallbackBench(t)...)
	results = append(results, runMixFallbackBench(t)...)
	results = append(results, runMixFailbackBench(t)...)
	results = append(results, runMixThirdProtocolFailbackBench(t)...)
	results = append(results, runMixProbeOverheadBench(t)...)

	logBytes := int64(0)
	logInfo, err := os.Stat(logFile)
	if err == nil {
		logBytes = logInfo.Size()
	} else if !os.IsNotExist(err) {
		requireNoError(t, err)
	}
	for i := range results {
		results[i].LogBytes = logBytes
	}

	writeMixBenchResults(t, outputDir, results)
}

func runSingleProtocolStableBench(t *testing.T) []mixBenchResult {
	t.Helper()
	cases := []struct {
		protocol string
		token    string
	}{
		{protocol: "tcp", token: "tcp://tcppass"},
		{protocol: "kcp", token: "kcp://kcppass"},
		{protocol: "quic", token: "quic://quicpass"},
		{protocol: "wss", token: "wss://wsspass"},
		{protocol: "ss", token: "ss://aes-256-gcm:sspass"},
		{protocol: "ssh", token: "ssh://user:sshpass"},
	}

	results := make([]mixBenchResult, 0, len(cases))
	for _, tc := range cases {
		if tc.protocol == "ss" {
			t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
		}
		server := startBenchServer(t, tc.token)
		clients, onlineAt, peakOnline := startAndWaitBenchClients(t, server.svc, server.mixPort, tc.token, tc.protocol+"-stable", tc.protocol, 100, 15*time.Second)
		time.Sleep(2 * time.Second)
		steadyOnline := countOnlineClients(server.svc, clients)

		result := buildBenchResult(
			"single-protocol-long",
			tc.protocol,
			100,
			1,
			clients,
			onlineAt,
			0,
			0,
			2*time.Second,
			fmt.Sprintf("steady online=%d, final online=%d", peakOnline, steadyOnline),
		)
		result.PeakOnlineObserved = peakOnline
		results = append(results, result)

		stopBenchClients(clients)
		server.Close()
	}
	return results
}

func runSingleProtocolBurstBench(t *testing.T) []mixBenchResult {
	t.Helper()
	cases := []struct {
		protocol string
		token    string
	}{
		{protocol: "tcp", token: "tcp://tcppass"},
		{protocol: "kcp", token: "kcp://kcppass"},
		{protocol: "quic", token: "quic://quicpass"},
		{protocol: "wss", token: "wss://wsspass"},
		{protocol: "ss", token: "ss://aes-256-gcm:sspass"},
		{protocol: "ssh", token: "ssh://user:sshpass"},
	}

	results := make([]mixBenchResult, 0, len(cases))
	for _, tc := range cases {
		if tc.protocol == "ss" {
			t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
		}
		server := startBenchServer(t, tc.token)
		totalClients := make([]*benchClientRunner, 0, 300)
		allOnlineAt := make(map[string]time.Time, 300)
		for cycle := 0; cycle < 3; cycle++ {
			prefix := fmt.Sprintf("%s-burst-%d", tc.protocol, cycle)
			clients, onlineAt, _ := startAndWaitBenchClients(t, server.svc, server.mixPort, tc.token, prefix, tc.protocol, 100, 15*time.Second)
			totalClients = append(totalClients, clients...)
			for key, ts := range onlineAt {
				allOnlineAt[key] = ts
			}
			stopBenchClients(clients)
		}
		results = append(results, buildBenchResult(
			"single-protocol-short-burst",
			tc.protocol,
			100,
			3,
			totalClients,
			allOnlineAt,
			0,
			0,
			0,
			"3 cycles of 100 short-lived control connections",
		))
		server.Close()
	}
	return results
}

func runMixStableScaleBench(t *testing.T) []mixBenchResult {
	t.Helper()
	token := "tcp://tcppass,ss://aes-256-gcm:sspass"
	server := startBenchServer(t, token)
	defer server.Close()

	results := make([]mixBenchResult, 0, 3)
	for _, concurrency := range []int{100, 500, 1000} {
		prefix := fmt.Sprintf("mix-stable-%d", concurrency)
		clients, onlineAt, peakOnline := startAndWaitBenchClients(t, server.svc, server.mixPort, token, prefix, "tcp", concurrency, 20*time.Second)
		result := buildBenchResult(
			"mix-primary-stable",
			"tcp",
			concurrency,
			1,
			clients,
			onlineAt,
			0,
			0,
			time.Second,
			fmt.Sprintf("peak online=%d", peakOnline),
		)
		result.PeakOnlineObserved = peakOnline
		results = append(results, result)
		stopBenchClients(clients)
	}
	return results
}

func runMixSoakBench(t *testing.T) []mixBenchResult {
	t.Helper()

	token := "tcp://tcppass,ss://aes-256-gcm:sspass"
	server := startBenchServer(t, token)
	defer server.Close()

	clients, onlineAt, peakOnline := startAndWaitBenchClients(t, server.svc, server.mixPort, token, "mix-soak", "tcp", 100, 20*time.Second)
	holdDuration := 15 * time.Second
	time.Sleep(holdDuration)
	finalOnline := countOnlineClients(server.svc, clients)

	result := buildBenchResult(
		"mix-primary-soak",
		"tcp",
		100,
		1,
		clients,
		onlineAt,
		0,
		0,
		holdDuration,
		fmt.Sprintf("peak online=%d, final online after soak=%d", peakOnline, finalOnline),
	)
	result.PeakOnlineObserved = peakOnline
	stopBenchClients(clients)
	return []mixBenchResult{result}
}

func runMixFallbackBench(t *testing.T) []mixBenchResult {
	t.Helper()
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
	serverToken := "ss://aes-256-gcm:sspass"
	clientToken := "tcp://wrong,ss://aes-256-gcm:sspass"
	server := startBenchServer(t, serverToken)
	defer server.Close()

	startedAt := time.Now()
	clients, onlineAt, _ := startAndWaitBenchClients(t, server.svc, server.mixPort, clientToken, "mix-fallback", "ss", 100, 20*time.Second)
	fallbackLatency := maxLatencySince(clients, onlineAt, startedAt)

	result := buildBenchResult(
		"mix-fallback",
		"ss",
		100,
		1,
		clients,
		onlineAt,
		fallbackLatency,
		0,
		0,
		"client token tcp://wrong,ss://..., fallback timing scaled to 100ms/200ms/3",
	)
	stopBenchClients(clients)
	return []mixBenchResult{result}
}

func runMixIntermittentFallbackBench(t *testing.T) []mixBenchResult {
	t.Helper()
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
	serverToken := "tcp://tcppass,ss://aes-256-gcm:sspass"
	server := startBenchServer(t, serverToken)
	defer server.Close()

	clients, _, peakOnline := startAndWaitBenchClients(t, server.svc, server.mixPort, serverToken, "mix-intermittent-fallback", "tcp", 100, 20*time.Second)

	server.svc.mixConfig.protocols[v1.MixProtocolTCP] = v1.MixProtocolConfig{
		Protocol: v1.MixProtocolTCP,
		Password: "wrong-tcp",
	}
	faultAt := time.Now()
	forceReconnectBenchClients(server.svc, clients)
	onlineOnSS, _ := waitForBenchClientsProtocol(server.svc, clients, "ss", 20*time.Second)

	result := buildBenchResult(
		"mix-intermittent-fallback",
		"ss",
		100,
		1,
		clients,
		onlineOnSS,
		maxLatencySince(clients, onlineOnSS, faultAt),
		0,
		0,
		fmt.Sprintf("clients first stabilized on tcp with peak online=%d before forced reconnect", peakOnline),
	)
	stopBenchClients(clients)
	return []mixBenchResult{result}
}

func runMixFailbackBench(t *testing.T) []mixBenchResult {
	t.Helper()
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
	serverToken := "tcp://tcppass,ss://aes-256-gcm:sspass"
	server := startBenchServer(t, serverToken)
	defer server.Close()

	server.svc.mixConfig.protocols[v1.MixProtocolTCP] = v1.MixProtocolConfig{
		Protocol: v1.MixProtocolTCP,
		Password: "wrong-tcp",
	}

	clients, onlineOnSS, _ := startAndWaitBenchClients(t, server.svc, server.mixPort, serverToken, "mix-failback", "ss", 100, 20*time.Second)
	restoreAt := time.Now()
	server.svc.mixConfig.protocols[v1.MixProtocolTCP] = v1.MixProtocolConfig{
		Protocol: v1.MixProtocolTCP,
		Password: "tcppass",
	}
	onlineOnTCP, _ := waitForBenchClientsProtocol(server.svc, clients, "tcp", 20*time.Second)

	result := buildBenchResult(
		"mix-failback",
		"tcp",
		100,
		1,
		clients,
		onlineOnTCP,
		0,
		maxLatencySince(clients, onlineOnTCP, restoreAt),
		0,
		fmt.Sprintf("all clients first converged on ss=%d before restore", len(onlineOnSS)),
	)
	stopBenchClients(clients)
	return []mixBenchResult{result}
}

func runMixThirdProtocolFailbackBench(t *testing.T) []mixBenchResult {
	t.Helper()
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
	serverToken := "tcp://tcppass,wss://wsspass,ss://aes-256-gcm:sspass"
	server := startBenchServer(t, serverToken)
	defer server.Close()

	server.svc.mixConfig.protocols[v1.MixProtocolTCP] = v1.MixProtocolConfig{
		Protocol: v1.MixProtocolTCP,
		Password: "wrong-tcp",
	}
	server.svc.mixConfig.protocols[v1.MixProtocolWSS] = v1.MixProtocolConfig{
		Protocol: v1.MixProtocolWSS,
		Password: "wrong-wss",
	}

	clients, onlineOnSS, _ := startAndWaitBenchClients(t, server.svc, server.mixPort, serverToken, "mix-third-failback", "ss", 100, 25*time.Second)
	restoreAt := time.Now()
	server.svc.mixConfig.protocols[v1.MixProtocolTCP] = v1.MixProtocolConfig{
		Protocol: v1.MixProtocolTCP,
		Password: "tcppass",
	}
	onlineOnTCP, _ := waitForBenchClientsProtocol(server.svc, clients, "tcp", 20*time.Second)

	result := buildBenchResult(
		"mix-third-to-first-failback",
		"tcp",
		100,
		1,
		clients,
		onlineOnTCP,
		0,
		maxLatencySince(clients, onlineOnTCP, restoreAt),
		0,
		fmt.Sprintf("all clients first converged on ss=%d before restoring tcp", len(onlineOnSS)),
	)
	stopBenchClients(clients)
	return []mixBenchResult{result}
}

func runMixProbeOverheadBench(t *testing.T) []mixBenchResult {
	t.Helper()
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
	serverToken := "tcp://tcppass,ss://aes-256-gcm:sspass"
	server := startBenchServer(t, serverToken)
	defer server.Close()

	server.svc.mixConfig.protocols[v1.MixProtocolTCP] = v1.MixProtocolConfig{
		Protocol: v1.MixProtocolTCP,
		Password: "wrong-tcp",
	}

	clients, onlineAt, _ := startAndWaitBenchClients(t, server.svc, server.mixPort, serverToken, "mix-probe", "ss", 100, 20*time.Second)
	holdDuration := 3 * time.Second
	time.Sleep(holdDuration)

	probeCount := 100 * int(holdDuration/(200*time.Millisecond))
	result := buildBenchResult(
		"mix-failback-probe-overhead",
		"ss",
		100,
		1,
		clients,
		onlineAt,
		0,
		0,
		holdDuration,
		fmt.Sprintf("approx probe attempts=%d with scaled 200ms failback interval", probeCount),
	)
	result.ApproxProbeCount = probeCount
	stopBenchClients(clients)
	return []mixBenchResult{result}
}

func startBenchServer(t *testing.T, token string) *benchServerRunner {
	t.Helper()

	mixPort, releaseMixPort := reserveDualStackPort(t)
	bindPort := getFreePort(t)
	serverCfg := &v1.ServerConfig{
		BindAddr:    "127.0.0.1",
		BindPort:    bindPort,
		MixBindPort: mixPort,
		MixToken:    token,
	}
	requireNoError(t, serverCfg.Complete())
	releaseMixPort()

	serverSvc, err := NewService(serverCfg)
	requireNoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		serverSvc.Run(ctx)
		close(done)
	}()

	return &benchServerRunner{
		svc:      serverSvc,
		cancel:   cancel,
		done:     done,
		mixPort:  mixPort,
		bindPort: bindPort,
	}
}

func (s *benchServerRunner) Close() {
	s.cancel()
	_ = s.svc.Close()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}

func startAndWaitBenchClients(
	t *testing.T,
	serverSvc *Service,
	mixPort int,
	token string,
	prefix string,
	expectedProtocol string,
	concurrency int,
	timeout time.Duration,
) ([]*benchClientRunner, map[string]time.Time, int) {
	t.Helper()

	clients := make([]*benchClientRunner, 0, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	startErrs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			clientID := fmt.Sprintf("%s-%04d", prefix, i)
			clientCfg := &v1.ClientCommonConfig{
				ServerAddr:    "127.0.0.1",
				MixBindPort:   mixPort,
				MixToken:      token,
				ClientID:      clientID,
				LoginFailExit: lo.ToPtr(false),
			}
			clientCfg.Transport.DialServerTimeout = 2
			requireNoError(t, clientCfg.Complete())

			clientSvc, err := clientpkg.NewService(clientpkg.ServiceOptions{
				Common:                 clientCfg,
				ConfigSourceAggregator: source.NewAggregator(source.NewConfigSource()),
			})
			if err != nil {
				startErrs <- err
				return
			}

			ctx, cancel := context.WithCancel(context.Background())
			started := time.Now()
			go clientSvc.Run(ctx)

			mu.Lock()
			clients = append(clients, &benchClientRunner{
				key:     clientID,
				svc:     clientSvc,
				cancel:  cancel,
				started: started,
			})
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	close(startErrs)
	for err := range startErrs {
		requireNoError(t, err)
	}

	onlineAt, peakOnline := waitForBenchClientsProtocol(serverSvc, clients, expectedProtocol, timeout)
	return clients, onlineAt, peakOnline
}

func waitForBenchClientsProtocol(
	serverSvc *Service,
	clients []*benchClientRunner,
	expectedProtocol string,
	timeout time.Duration,
) (map[string]time.Time, int) {
	targets := make(map[string]time.Time, len(clients))
	for _, client := range clients {
		targets[client.key] = client.started
	}

	onlineAt := make(map[string]time.Time, len(clients))
	peakOnline := 0
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		now := time.Now()
		onlineCount := 0
		for _, info := range serverSvc.clientRegistry.List() {
			started, ok := targets[info.Key]
			if !ok {
				continue
			}
			if info.Online {
				onlineCount++
			}
			if info.Online && info.SelectedProtocol == expectedProtocol {
				if _, exists := onlineAt[info.Key]; !exists {
					if now.Before(started) {
						now = started
					}
					onlineAt[info.Key] = now
				}
			}
		}
		if onlineCount > peakOnline {
			peakOnline = onlineCount
		}
		if len(onlineAt) == len(targets) {
			return onlineAt, peakOnline
		}
		time.Sleep(25 * time.Millisecond)
	}
	return onlineAt, peakOnline
}

func stopBenchClients(clients []*benchClientRunner) {
	for _, client := range clients {
		client.cancel()
		client.svc.Close()
	}
	time.Sleep(100 * time.Millisecond)
}

func forceReconnectBenchClients(serverSvc *Service, clients []*benchClientRunner) {
	targets := make(map[string]struct{}, len(clients))
	for _, client := range clients {
		targets[client.key] = struct{}{}
	}
	for _, info := range serverSvc.clientRegistry.List() {
		if _, ok := targets[info.Key]; !ok || info.RunID == "" {
			continue
		}
		if ctl, ok := serverSvc.ctlManager.GetByID(info.RunID); ok {
			_ = ctl.Close()
		}
	}
}

func countOnlineClients(serverSvc *Service, clients []*benchClientRunner) int {
	targets := make(map[string]struct{}, len(clients))
	for _, client := range clients {
		targets[client.key] = struct{}{}
	}
	count := 0
	for _, info := range serverSvc.clientRegistry.List() {
		if _, ok := targets[info.Key]; ok && info.Online {
			count++
		}
	}
	return count
}

func buildBenchResult(
	scenario string,
	protocol string,
	concurrency int,
	iterations int,
	clients []*benchClientRunner,
	onlineAt map[string]time.Time,
	fallbackLatency time.Duration,
	failbackLatency time.Duration,
	holdDuration time.Duration,
	notes string,
) mixBenchResult {
	latencies := make([]float64, 0, len(onlineAt))
	for _, client := range clients {
		if ts, ok := onlineAt[client.key]; ok {
			latencies = append(latencies, ts.Sub(client.started).Seconds()*1000)
		}
	}
	sort.Float64s(latencies)

	successes := len(onlineAt)
	result := mixBenchResult{
		Scenario:          scenario,
		Protocol:          protocol,
		Concurrency:       concurrency,
		Iterations:        iterations,
		Attempts:          len(clients),
		Successes:         successes,
		SuccessRate:       ratio(successes, len(clients)),
		AvgConnectMs:      averageFloat(latencies),
		P95ConnectMs:      percentile(latencies, 0.95),
		FallbackLatencyMs: fallbackLatency.Seconds() * 1000,
		FailbackLatencyMs: failbackLatency.Seconds() * 1000,
		HoldSeconds:       holdDuration.Seconds(),
		Notes:             notes,
	}
	return result
}

func maxLatencySince(clients []*benchClientRunner, onlineAt map[string]time.Time, since time.Time) time.Duration {
	maxDuration := time.Duration(0)
	for _, ts := range onlineAt {
		if ts.After(since) {
			if d := ts.Sub(since); d > maxDuration {
				maxDuration = d
			}
		}
	}
	return maxDuration
}

func averageFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		return values[0]
	}
	if p >= 1 {
		return values[len(values)-1]
	}
	index := int(math.Ceil(p*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func ratio(successes, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(successes) / float64(total)
}

func writeMixBenchResults(t *testing.T, outputDir string, results []mixBenchResult) {
	t.Helper()

	sort.Slice(results, func(i, j int) bool {
		if results[i].Scenario != results[j].Scenario {
			return results[i].Scenario < results[j].Scenario
		}
		if results[i].Protocol != results[j].Protocol {
			return results[i].Protocol < results[j].Protocol
		}
		return results[i].Concurrency < results[j].Concurrency
	})

	jsonBytes, err := json.MarshalIndent(results, "", "  ")
	requireNoError(t, err)
	requireNoError(t, os.WriteFile(filepath.Join(outputDir, "bench-summary.json"), jsonBytes, 0o644))

	var builder strings.Builder
	builder.WriteString("| scenario | protocol | concurrency | attempts | success rate | avg connect ms | p95 connect ms | fallback ms | failback ms | hold s | peak online | probe count | log bytes | notes |\n")
	builder.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
	for _, result := range results {
		builder.WriteString(fmt.Sprintf(
			"| %s | %s | %d | %d | %.2f%% | %.2f | %.2f | %.2f | %.2f | %.2f | %d | %d | %d | %s |\n",
			result.Scenario,
			result.Protocol,
			result.Concurrency,
			result.Attempts,
			result.SuccessRate*100,
			result.AvgConnectMs,
			result.P95ConnectMs,
			result.FallbackLatencyMs,
			result.FailbackLatencyMs,
			result.HoldSeconds,
			result.PeakOnlineObserved,
			result.ApproxProbeCount,
			result.LogBytes,
			result.Notes,
		))
	}
	requireNoError(t, os.WriteFile(filepath.Join(outputDir, "bench-summary.md"), []byte(builder.String()), 0o644))
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func reserveDualStackPort(t *testing.T) (int, func()) {
	t.Helper()

	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	requireNoError(t, err)
	port := tcpLn.Addr().(*net.TCPAddr).Port

	udpConn, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
	requireNoError(t, err)

	return port, func() {
		_ = udpConn.Close()
		_ = tcpLn.Close()
	}
}
