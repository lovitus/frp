// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wizard

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type runnerCall struct {
	cwd        string
	executable string
	args       []string
	wait       time.Duration
}

type fakeRunner struct {
	runOutput   string
	runErr      error
	smokeResult SmokeResult
	smokeErr    error
	runCalls    []runnerCall
	smokeCalls  []runnerCall
}

func (r *fakeRunner) Run(_ context.Context, cwd string, executable string, args []string) (string, error) {
	r.runCalls = append(r.runCalls, runnerCall{cwd: cwd, executable: executable, args: append([]string(nil), args...)})
	return r.runOutput, r.runErr
}

func (r *fakeRunner) Smoke(_ context.Context, cwd string, executable string, args []string, wait time.Duration) (SmokeResult, error) {
	r.smokeCalls = append(r.smokeCalls, runnerCall{cwd: cwd, executable: executable, args: append([]string(nil), args...), wait: wait})
	return r.smokeResult, r.smokeErr
}

func testOptions(t *testing.T, stdin string, runner *fakeRunner) (Options, *strings.Builder, *strings.Builder) {
	t.Helper()
	var stdout, stderr strings.Builder
	return Options{
		Stdin:      strings.NewReader(stdin),
		Stdout:     &stdout,
		Stderr:     &stderr,
		Cwd:        t.TempDir(),
		Executable: filepath.Join(t.TempDir(), "frps"),
		Version:    "0.68.1-mix.16",
		Platform:   "linux",
		OpenTTY: func() (io.ReadCloser, error) {
			return nil, errors.New("no tty")
		},
		Runner: runner,
	}, &stdout, &stderr
}

func TestResolveConfigPath(t *testing.T) {
	cwd := t.TempDir()
	got, err := resolveConfigPath(cwd, "", false, "frps.toml")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(cwd, "frps.toml"), got)

	got, err = resolveConfigPath(cwd, "custom.toml", true, "frps.toml")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(cwd, "custom.toml"), got)

	abs := filepath.Join(t.TempDir(), "custom.toml")
	got, err = resolveConfigPath(cwd, abs, true, "frps.toml")
	require.NoError(t, err)
	require.Equal(t, abs, got)
}

func TestRunServerExistingConfigDoesNotReadStdinOrOverwrite(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, _, _ := testOptions(t, "", runner)
	target := filepath.Join(opts.Cwd, "frps.toml")
	require.NoError(t, os.WriteFile(target, []byte("keep"), 0o600))

	err := RunServer(opts, ServerConfig{})
	require.ErrorContains(t, err, "already exists")
	content, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, "keep", string(content))
	require.Empty(t, runner.runCalls)
	require.Empty(t, runner.smokeCalls)
}

func TestRunServerGeneratesConfigVerifiesSmokesAndPrintsPinnedCommands(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, stdout, _ := testOptions(t, "\n\nsecret\n\n\n\n", runner)

	err := RunServer(opts, ServerConfig{})
	require.NoError(t, err)

	target := filepath.Join(opts.Cwd, "frps.toml")
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, `bindPort = 7000
mixBindPort = 7001
mixToken = "ss://chacha20-ietf-poly1305:secret,kcp://secret,ssh://frp:secret"

webServer.addr = "0.0.0.0"
webServer.port = 7002
webServer.user = "admin"
webServer.password = "secret"
`, string(content))
	require.Len(t, runner.runCalls, 1)
	require.Equal(t, []string{"verify", "-c", target}, runner.runCalls[0].args)
	require.Len(t, runner.smokeCalls, 1)
	require.Equal(t, []string{"-c", target}, runner.smokeCalls[0].args)
	require.Equal(t, 2*time.Second, runner.smokeCalls[0].wait)
	require.Contains(t, stdout.String(), "--release-tag v0.68.1-mix.16")
	require.Contains(t, stdout.String(), "--mix-token-b64")
	require.Contains(t, stdout.String(), "install-frpc.sh | sh -s --")
	require.Contains(t, stdout.String(), "Unix/macOS/Linux local frpc wizard command")
	require.Contains(t, stdout.String(), "./frpc --wizard")
	require.Contains(t, stdout.String(), "Windows PowerShell local frpc wizard command")
	require.Contains(t, stdout.String(), `& '.\frpc.exe'`)
	require.NotContains(t, stdout.String(), filepath.Join(filepath.Dir(opts.Executable), "frpc"))
	require.Contains(t, stdout.String(), "Smoke start check passed.")
}

func TestRunServerDashboardPortOverflowFallback(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, _, _ := testOptions(t, "\n65535\nsecret\n\n\n\n", runner)

	err := RunServer(opts, ServerConfig{})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(opts.Cwd, "frps.toml"))
	require.NoError(t, err)
	require.Contains(t, string(content), "webServer.port = 7501")
}

func TestRunServerRejectsCommaInConnectionPassword(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, _, stderr := testOptions(t, "\n\nbad,pass\nsecret\n\n\n\n", runner)

	err := RunServer(opts, ServerConfig{})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(opts.Cwd, "frps.toml"))
	require.NoError(t, err)
	require.Contains(t, string(content), `mixToken = "ss://chacha20-ietf-poly1305:secret,kcp://secret,ssh://frp:secret"`)
	require.NotContains(t, string(content), "bad,pass")
	require.Contains(t, stderr.String(), "Password cannot contain comma")
}

func TestDefaultMixTokenEscapesOnlyAtTOMLRender(t *testing.T) {
	password := `pa\ss"word`
	token := buildDefaultMixToken(password)
	require.Equal(t, `ss://chacha20-ietf-poly1305:pa\ss"word,kcp://pa\ss"word,ssh://frp:pa\ss"word`, token)

	content := renderServerConfig(serverRenderConfig{
		BindPort:          7000,
		MixBindPort:       7001,
		MixToken:          token,
		DashboardAddr:     "0.0.0.0",
		DashboardPort:     7002,
		DashboardUser:     "admin",
		DashboardPassword: password,
	})
	require.Contains(t, content, `mixToken = "ss://chacha20-ietf-poly1305:pa\\ss\"word,kcp://pa\\ss\"word,ssh://frp:pa\\ss\"word"`)
	require.Contains(t, content, `webServer.password = "pa\\ss\"word"`)
}

func TestRunClientImplicitPathIgnoresRuntimeFrpcIniDefault(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, _, _ := testOptions(t, "7001\nsecret\n127.0.0.1\nedge-1\n", runner)
	opts.Executable = filepath.Join(filepath.Dir(opts.Executable), "frpc")

	err := RunClient(opts, ClientConfig{ConfigFile: "./frpc.ini", ConfigFileExplicit: false})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(opts.Cwd, "frpc.toml"))
	require.NoFileExists(t, filepath.Join(opts.Cwd, "frpc.ini"))
}

func TestRunClientRejectsCommaInConnectionPassword(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, _, stderr := testOptions(t, "7001\nbad,pass\nsecret\n127.0.0.1\nedge-1\n", runner)

	err := RunClient(opts, ClientConfig{})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(opts.Cwd, "frpc.toml"))
	require.NoError(t, err)
	require.Contains(t, string(content), `mixToken = "ss://chacha20-ietf-poly1305:secret,kcp://secret,ssh://frp:secret"`)
	require.NotContains(t, string(content), "bad,pass")
	require.Contains(t, stderr.String(), "Password cannot contain comma")
}

func TestRunClientPresetB64SkipsMixPromptAndPassword(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, stdout, _ := testOptions(t, "example.com\nedge-1\n", runner)
	opts.Executable = filepath.Join(filepath.Dir(opts.Executable), "frpc")

	err := RunClient(opts, ClientConfig{
		MixBindPort: "7002",
		MixToken:    "ignored",
		MixTokenB64: "c3M6Ly94",
	})
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(opts.Cwd, "frpc.toml"))
	require.NoError(t, err)
	require.Equal(t, `serverAddr = "example.com"
mixBindPort = 7002
mixToken = "ss://x"
# mixFallbackHosts = "backup-a.example.com,backup-b.example.com:7002"

clientID = "edge-1"
allowGatewayTunnels = true
mixAllowGateway = true
loginFailExit = false
`, string(content))
	require.Contains(t, stdout.String(), "Configure frpc (preset mode from frps command)")
	require.NotContains(t, stdout.String(), "Connection password")
	require.NotContains(t, stdout.String(), "mixBindPort [")
}

func TestRunClientPresetAllValuesPrefilledDoesNotReadStdin(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, stdout, _ := testOptions(t, "", runner)
	opts.Executable = filepath.Join(filepath.Dir(opts.Executable), "frpc")

	err := RunClient(opts, ClientConfig{
		ServerAddr:  "example.com",
		MixBindPort: "7002",
		MixTokenB64: "c3M6Ly94",
		ClientID:    "edge-1",
	})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "Configure frpc (preset mode from frps command)")
	require.NotContains(t, stdout.String(), "serverAddr (frps IP/domain):")
	require.NotContains(t, stdout.String(), "clientID:")
}

func TestRunClientB64DecodeError(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, _, _ := testOptions(t, "", runner)

	err := RunClient(opts, ClientConfig{MixTokenB64: "not-base64"})
	require.ErrorContains(t, err, "failed to decode --mix-token-b64")
	require.Empty(t, runner.runCalls)
	require.Empty(t, runner.smokeCalls)
}

func TestRunClientUnexpectedEOF(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, _, _ := testOptions(t, "", runner)

	err := RunClient(opts, ClientConfig{})
	require.ErrorContains(t, err, "Input aborted: unexpected EOF")
	require.Empty(t, runner.runCalls)
	require.Empty(t, runner.smokeCalls)
}

func TestRunClientSmokeEarlyExitWarnsButSucceeds(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: false, Output: "recent log\n"}}
	opts, stdout, stderr := testOptions(t, "7001\nsecret\n127.0.0.1\nedge-1\n", runner)

	err := RunClient(opts, ClientConfig{})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "Smoke start check warning: frpc exited quickly")
	require.Contains(t, stdout.String(), "You can still run manually after checking server reachability.")
	require.Contains(t, stderr.String(), "recent log")
}

func TestRunServerSmokeEarlyExitIsFatal(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: false, Output: "server failed\n"}}
	opts, stdout, stderr := testOptions(t, "\n\nsecret\n\n\n\n", runner)

	err := RunServer(opts, ServerConfig{})
	require.ErrorContains(t, err, "smoke start check failed")
	require.Contains(t, stdout.String(), "Smoke start check failed. Recent log:")
	require.Contains(t, stderr.String(), "server failed")
}

func TestShellQuoting(t *testing.T) {
	require.Equal(t, "'/tmp/frp dir/frps' -c '/tmp/cfg dir/frps.toml'", shellCommand("linux", "/tmp/frp dir/frps", "-c", "/tmp/cfg dir/frps.toml"))
	require.Equal(t, "& 'C:\\Program Files\\frp\\frps.exe' '-c' 'C:\\frp configs\\frps.toml'", shellCommand("windows", `C:\Program Files\frp\frps.exe`, "-c", `C:\frp configs\frps.toml`))
	require.Equal(t, "'a'\\''b'", posixQuote("a'b"))
	require.Equal(t, "'a''b'", powerShellQuote("a'b"))
}

func TestNextStepsUnpinnedForDevVersion(t *testing.T) {
	runner := &fakeRunner{smokeResult: SmokeResult{StillRunning: true}}
	opts, stdout, _ := testOptions(t, "\n\nsecret\n\n\n\n", runner)
	opts.Version = "dev"

	err := RunServer(opts, ServerConfig{})
	require.NoError(t, err)
	require.NotContains(t, stdout.String(), "--release-tag")
	require.Contains(t, stdout.String(), DefaultRepo)
	require.Contains(t, stdout.String(), DefaultRawBase)
}
