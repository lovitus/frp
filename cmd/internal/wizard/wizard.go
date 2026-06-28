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
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultRepo = "lovitus/frp"
	// Intentionally track the maintained mix quick-deploy branch instead of
	// pinning raw script URLs to the running binary's release tag. The wizard's
	// online fallback is meant to keep using the current branch-hosted scripts.
	DefaultRawBase = "https://raw.githubusercontent.com/lovitus/frp/codex/mix-transport-release/hack/quick-deploy"

	defaultServerConfigName = "frps.toml"
	defaultClientConfigName = "frpc.toml"
	defaultBindPort         = 7000
	defaultMixBindPort      = 7001
	defaultDashboardAddr    = "0.0.0.0"
	defaultDashboardUser    = "admin"
)

var releaseVersionRE = regexp.MustCompile(`^\d+\.\d+\.\d+(-[A-Za-z][A-Za-z0-9]*(\.[A-Za-z0-9]+)*)?$`)

type Options struct {
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Cwd        string
	Executable string
	Version    string
	Platform   string
	OpenTTY    func() (io.ReadCloser, error)
	Runner     Runner
}

type ServerConfig struct {
	ConfigFile         string
	ConfigFileExplicit bool
}

type ClientConfig struct {
	ConfigFile         string
	ConfigFileExplicit bool
	ConfigDir          string
	ServerAddr         string
	MixBindPort        string
	MixToken           string
	MixTokenB64        string
	ClientID           string
}

type Runner interface {
	Run(ctx context.Context, cwd string, executable string, args []string) (string, error)
	Smoke(ctx context.Context, cwd string, executable string, args []string, wait time.Duration) (SmokeResult, error)
}

type SmokeResult struct {
	StillRunning bool
	Output       string
}

type execRunner struct{}

func RuntimeOptions(stdin io.Reader, stdout, stderr io.Writer, executable, version string) Options {
	cwd, _ := os.Getwd()
	if executable == "" {
		executable, _ = os.Executable()
	}
	return Options{
		Stdin:      stdin,
		Stdout:     stdout,
		Stderr:     stderr,
		Cwd:        cwd,
		Executable: executable,
		Version:    version,
		Platform:   runtime.GOOS,
		OpenTTY:    defaultOpenTTY,
		Runner:     execRunner{},
	}
}

func RunServer(opts Options, cfg ServerConfig) error {
	opts = opts.withDefaults()
	target, err := resolveConfigPath(opts.Cwd, cfg.ConfigFile, cfg.ConfigFileExplicit, defaultServerConfigName)
	if err != nil {
		return err
	}
	if err := guardTargetPath(target); err != nil {
		return err
	}

	p, closeInput := opts.prompter()
	defer closeInput()

	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Configure frps (press Enter to accept defaults)")

	bindPort, err := p.promptPort("bindPort", defaultBindPort)
	if err != nil {
		return err
	}
	mixBindPort, err := p.promptPort("mixBindPort", defaultMixBindPort)
	if err != nil {
		return err
	}
	connectPassword, err := p.promptMixPassword("Connection password (used for ss/kcp/ssh)", "")
	if err != nil {
		return err
	}
	dashboardAddr, err := p.promptLine("dashboard/webServer addr", defaultDashboardAddr)
	if err != nil {
		return err
	}
	defaultDashboardPort := mixBindPort + 1
	if defaultDashboardPort > 65535 {
		defaultDashboardPort = 7501
	}
	dashboardPort, err := p.promptPort("dashboard/webServer port", defaultDashboardPort)
	if err != nil {
		return err
	}
	dashboardPassword, err := p.promptPassword("dashboard password", connectPassword)
	if err != nil {
		return err
	}

	mixToken := buildDefaultMixToken(connectPassword)
	content := renderServerConfig(serverRenderConfig{
		BindPort:          bindPort,
		MixBindPort:       mixBindPort,
		MixToken:          mixToken,
		DashboardAddr:     dashboardAddr,
		DashboardPort:     dashboardPort,
		DashboardUser:     defaultDashboardUser,
		DashboardPassword: dashboardPassword,
	})
	if err := writeConfigExclusive(target, content); err != nil {
		return err
	}
	if err := verifyConfig(opts, target); err != nil {
		return err
	}
	if err := smokeServer(opts, target); err != nil {
		return err
	}
	printServerNextSteps(opts, target, mixBindPort, mixToken)
	return nil
}

func RunClient(opts Options, cfg ClientConfig) error {
	opts = opts.withDefaults()
	if cfg.ConfigDir != "" {
		return errors.New("frpc --wizard is incompatible with --config_dir; remove --config_dir and rerun --wizard")
	}
	target, err := resolveConfigPath(opts.Cwd, cfg.ConfigFile, cfg.ConfigFileExplicit, defaultClientConfigName)
	if err != nil {
		return err
	}
	if err := guardTargetPath(target); err != nil {
		return err
	}

	p, closeInput := opts.prompter()
	defer closeInput()

	mixToken := cfg.MixToken
	if cfg.MixTokenB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(cfg.MixTokenB64)
		if err != nil {
			return fmt.Errorf("failed to decode --mix-token-b64 value: %w", err)
		}
		mixToken = string(decoded)
	}

	mixBindPort := 0
	if cfg.MixBindPort != "" {
		mixBindPort, err = parsePort(cfg.MixBindPort)
		if err != nil {
			return fmt.Errorf("invalid --mix-bind-port %q: expected 1..65535", cfg.MixBindPort)
		}
	}

	fmt.Fprintln(opts.Stdout)
	if mixToken == "" {
		fmt.Fprintln(opts.Stdout, "Configure frpc (standalone mode, press Enter to accept defaults)")
		if mixBindPort == 0 {
			mixBindPort, err = p.promptPort("mixBindPort", defaultMixBindPort)
			if err != nil {
				return err
			}
		}
		setupPassword, err := p.promptMixPassword("Connection password (used for ss/kcp/ssh)", "")
		if err != nil {
			return err
		}
		mixToken = buildDefaultMixToken(setupPassword)
	} else {
		fmt.Fprintln(opts.Stdout, "Configure frpc (preset mode from frps command)")
		if mixBindPort == 0 {
			mixBindPort, err = p.promptPort("mixBindPort", defaultMixBindPort)
			if err != nil {
				return err
			}
		}
	}

	serverAddr := cfg.ServerAddr
	if serverAddr == "" {
		serverAddr, err = p.promptLine("serverAddr (frps IP/domain)", "")
		if err != nil {
			return err
		}
	}
	clientID := cfg.ClientID
	if clientID == "" {
		clientID, err = p.promptLine("clientID", "")
		if err != nil {
			return err
		}
	}

	content := renderClientConfig(clientRenderConfig{
		ServerAddr:  serverAddr,
		MixBindPort: mixBindPort,
		MixToken:    mixToken,
		ClientID:    clientID,
	})
	if err := writeConfigExclusive(target, content); err != nil {
		return err
	}
	if err := verifyConfig(opts, target); err != nil {
		return err
	}
	if err := smokeClient(opts, target); err != nil {
		return err
	}
	printClientNextSteps(opts, target)
	return nil
}

func (o Options) withDefaults() Options {
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Cwd == "" {
		o.Cwd, _ = os.Getwd()
	}
	if o.Executable == "" {
		o.Executable, _ = os.Executable()
	}
	if o.Platform == "" {
		o.Platform = runtime.GOOS
	}
	if o.OpenTTY == nil {
		o.OpenTTY = defaultOpenTTY
	}
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	return o
}

func (o Options) prompter() (*prompter, func()) {
	if o.OpenTTY != nil {
		if tty, err := o.OpenTTY(); err == nil {
			return newPrompter(tty, o.Stdout, o.Stderr), func() { _ = tty.Close() }
		}
	}
	return newPrompter(o.Stdin, o.Stdout, o.Stderr), func() {}
}

type prompter struct {
	reader *bufio.Reader
	out    io.Writer
	err    io.Writer
}

func newPrompter(in io.Reader, out, errOut io.Writer) *prompter {
	return &prompter{reader: bufio.NewReader(in), out: out, err: errOut}
}

func (p *prompter) promptLine(label, defaultValue string) (string, error) {
	for {
		if defaultValue != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", label, defaultValue)
		} else {
			fmt.Fprintf(p.out, "%s: ", label)
		}
		value, err := p.readLine()
		if err != nil {
			return "", err
		}
		value = trimInput(value)
		if value == "" {
			if defaultValue != "" {
				return defaultValue, nil
			}
			fmt.Fprintln(p.err, "This value is required.")
			continue
		}
		return value, nil
	}
}

func (p *prompter) promptPassword(label, defaultValue string) (string, error) {
	for {
		if defaultValue != "" {
			fmt.Fprintf(p.out, "%s [press Enter to use default]: ", label)
		} else {
			fmt.Fprintf(p.out, "%s: ", label)
		}
		value, err := p.readLine()
		if err != nil {
			return "", err
		}
		value = trimInput(value)
		if value == "" {
			value = defaultValue
		}
		if value != "" {
			return value, nil
		}
		fmt.Fprintln(p.err, "Password cannot be empty.")
	}
}

func (p *prompter) promptMixPassword(label, defaultValue string) (string, error) {
	for {
		value, err := p.promptPassword(label, defaultValue)
		if err != nil {
			return "", err
		}
		if !strings.Contains(value, ",") {
			return value, nil
		}
		fmt.Fprintln(p.err, "Password cannot contain comma (,).")
	}
}

func (p *prompter) promptPort(label string, defaultValue int) (int, error) {
	for {
		value, err := p.promptLine(label, strconv.Itoa(defaultValue))
		if err != nil {
			return 0, err
		}
		port, err := parsePort(value)
		if err == nil {
			return port, nil
		}
		fmt.Fprintf(p.err, "Invalid port '%s'. Expected 1..65535.\n", value)
	}
}

func (p *prompter) readLine() (string, error) {
	line, err := p.reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) && line != "" {
			return line, nil
		}
		if errors.Is(err, io.EOF) {
			return "", errors.New("Input aborted: unexpected EOF")
		}
		return "", fmt.Errorf("Input aborted: %w", err)
	}
	return line, nil
}

func resolveConfigPath(cwd, cfgFile string, explicit bool, defaultName string) (string, error) {
	target := cfgFile
	if !explicit {
		target = defaultName
	}
	if target == "" {
		target = defaultName
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}
	return filepath.Join(cwd, target), nil
}

func guardTargetPath(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("config target %s already exists as a directory; delete or rename it and rerun --wizard", path)
		}
		return fmt.Errorf("config file %s already exists; delete or rename it and rerun --wizard", path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot inspect config target %s: %w", path, err)
	}
	parent := filepath.Dir(path)
	info, err = os.Stat(parent)
	if err != nil {
		return fmt.Errorf("config directory %s is not available: %w", parent, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("config directory %s is not a directory", parent)
	}
	return nil
}

func writeConfigExclusive(path string, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("config file %s already exists; delete or rename it and rerun --wizard", path)
		}
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}
	defer f.Close()
	if _, err := io.WriteString(f, content); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}
	return nil
}

func verifyConfig(opts Options, configPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := opts.Runner.Run(ctx, opts.Cwd, opts.Executable, []string{"verify", "-c", configPath})
	if err != nil {
		fmt.Fprintln(opts.Stdout, "Config verification failed:")
		fmt.Fprint(opts.Stderr, output)
		return fmt.Errorf("config verification failed: %w", err)
	}
	return nil
}

func smokeServer(opts Options, configPath string) error {
	result, err := opts.Runner.Smoke(context.Background(), opts.Cwd, opts.Executable, []string{"-c", configPath}, 2*time.Second)
	if err != nil {
		return fmt.Errorf("smoke start check failed: %w", err)
	}
	if result.StillRunning {
		fmt.Fprintln(opts.Stdout, "Smoke start check passed.")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Smoke start check failed. Recent log:")
	fmt.Fprint(opts.Stderr, tailLines(result.Output, 60))
	return errors.New("smoke start check failed")
}

func smokeClient(opts Options, configPath string) error {
	result, err := opts.Runner.Smoke(context.Background(), opts.Cwd, opts.Executable, []string{"-c", configPath}, 3*time.Second)
	if err != nil {
		return fmt.Errorf("smoke start check failed: %w", err)
	}
	if result.StillRunning {
		fmt.Fprintln(opts.Stdout, "Smoke start check passed.")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Smoke start check warning: frpc exited quickly. Recent log:")
	fmt.Fprint(opts.Stderr, tailLines(result.Output, 60))
	fmt.Fprintln(opts.Stdout, "You can still run manually after checking server reachability.")
	return nil
}

func printServerNextSteps(opts Options, configPath string, mixBindPort int, mixToken string) {
	tokenB64 := base64.StdEncoding.EncodeToString([]byte(mixToken))
	releaseTag, pinned := releaseTag(opts.Version)
	frpcURL := DefaultRawBase + "/install-frpc.sh"
	frpcPSURL := DefaultRawBase + "/install-frpc.ps1"

	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Done. Generated config:")
	fmt.Fprintf(opts.Stdout, "  - %s\n", configPath)
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Local start command:")
	fmt.Fprintf(opts.Stdout, "  %s\n", shellCommand(opts.Platform, opts.Executable, "-c", configPath))
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Unix/macOS/Linux local frpc wizard command (preloaded mix settings):")
	fmt.Fprintf(opts.Stdout, "  %s\n", shellCommand("linux", "./frpc", "--wizard", "--mix-bind-port", strconv.Itoa(mixBindPort), "--mix-token-b64", tokenB64))
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Windows PowerShell local frpc wizard command (preloaded mix settings):")
	fmt.Fprintf(opts.Stdout, "  %s\n", shellCommand("windows", `.\frpc.exe`, "--wizard", "--mix-bind-port", strconv.Itoa(mixBindPort), "--mix-token-b64", tokenB64))
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Unix/macOS/Linux frpc quick-deploy command (preloaded mix settings):")
	fmt.Fprintf(opts.Stdout, "  %s\n", unixQuickDeployCommand("wget", frpcURL, pinned, releaseTag, mixBindPort, tokenB64))
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Curl alternative:")
	fmt.Fprintf(opts.Stdout, "  %s\n", unixQuickDeployCommand("curl", frpcURL, pinned, releaseTag, mixBindPort, tokenB64))
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Windows PowerShell frpc quick-deploy command (preloaded mix settings):")
	fmt.Fprintf(opts.Stdout, "  %s\n", powerShellQuickDeployCommand(frpcPSURL, pinned, releaseTag, mixBindPort, tokenB64))
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "When either quick-deploy command runs, it will ask only:")
	fmt.Fprintln(opts.Stdout, "  1) serverAddr (your frps public IP/domain)")
	fmt.Fprintln(opts.Stdout, "  2) clientID")
}

func printClientNextSteps(opts Options, configPath string) {
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Done. Generated config:")
	fmt.Fprintf(opts.Stdout, "  - %s\n", configPath)
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Local start command:")
	fmt.Fprintf(opts.Stdout, "  %s\n", shellCommand(opts.Platform, opts.Executable, "-c", configPath))
}

func (execRunner) Run(ctx context.Context, cwd string, executable string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (execRunner) Smoke(parent context.Context, cwd string, executable string, args []string, wait time.Duration) (SmokeResult, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var output bytes.Buffer
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = cwd
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return SmokeResult{}, err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		cancel()
		<-done
		return SmokeResult{StillRunning: true, Output: output.String()}, nil
	case <-done:
		return SmokeResult{StillRunning: false, Output: output.String()}, nil
	case <-parent.Done():
		cancel()
		<-done
		return SmokeResult{StillRunning: false, Output: output.String()}, parent.Err()
	}
}

func renderServerConfig(c serverRenderConfig) string {
	return fmt.Sprintf(`bindPort = %d
mixBindPort = %d
mixToken = "%s"

webServer.addr = "%s"
webServer.port = %d
webServer.user = "%s"
webServer.password = "%s"
`, c.BindPort, c.MixBindPort, tomlEscape(c.MixToken), tomlEscape(c.DashboardAddr), c.DashboardPort, tomlEscape(c.DashboardUser), tomlEscape(c.DashboardPassword))
}

func renderClientConfig(c clientRenderConfig) string {
	return fmt.Sprintf(`serverAddr = "%s"
mixBindPort = %d
mixToken = "%s"
# mixFallbackHosts = "backup-a.example.com,backup-b.example.com:7002"

clientID = "%s"
allowGatewayTunnels = true
mixAllowGateway = true
loginFailExit = false
`, tomlEscape(c.ServerAddr), c.MixBindPort, tomlEscape(c.MixToken), tomlEscape(c.ClientID))
}

type serverRenderConfig struct {
	BindPort          int
	MixBindPort       int
	MixToken          string
	DashboardAddr     string
	DashboardPort     int
	DashboardUser     string
	DashboardPassword string
}

type clientRenderConfig struct {
	ServerAddr  string
	MixBindPort int
	MixToken    string
	ClientID    string
}

func buildDefaultMixToken(password string) string {
	return fmt.Sprintf("ss://chacha20-ietf-poly1305:%s,kcp://%s,ssh://frp:%s", password, password, password)
}

func parsePort(value string) (int, error) {
	if value == "" {
		return 0, errors.New("empty port")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, errors.New("invalid port")
		}
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("invalid port")
	}
	return port, nil
}

func trimInput(value string) string {
	value = strings.TrimRight(value, "\r\n")
	return strings.TrimSpace(value)
}

func tomlEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func tailLines(value string, n int) string {
	if value == "" {
		return ""
	}
	lines := strings.SplitAfter(value, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "")
}

func releaseTag(version string) (string, bool) {
	version = strings.TrimSpace(version)
	lower := strings.ToLower(version)
	if version == "" || strings.Contains(lower, "dev") || strings.Contains(lower, "dirty") || strings.Contains(version, "-g") {
		return "", false
	}
	if !releaseVersionRE.MatchString(version) {
		return "", false
	}
	return "v" + version, true
}

func unixQuickDeployCommand(tool, url string, pinned bool, releaseTag string, mixBindPort int, tokenB64 string) string {
	prefix := ""
	switch tool {
	case "curl":
		prefix = "curl -fsSL " + posixQuote(url)
	default:
		prefix = "wget -O- " + posixQuote(url)
	}
	args := []string{"sh", "-s", "--", "--repo", DefaultRepo}
	if pinned {
		args = append(args, "--release-tag", releaseTag)
	}
	args = append(args, "--mix-bind-port", strconv.Itoa(mixBindPort), "--mix-token-b64", tokenB64)
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, posixQuote(arg))
	}
	return prefix + " | " + strings.Join(quoted, " ")
}

func powerShellQuickDeployCommand(url string, pinned bool, releaseTag string, mixBindPort int, tokenB64 string) string {
	parts := []string{
		"$env:FRP_REPO=" + powerShellQuote(DefaultRepo),
		"$env:FRP_MIX_BIND_PORT=" + powerShellQuote(strconv.Itoa(mixBindPort)),
		"$env:FRP_MIX_TOKEN_B64=" + powerShellQuote(tokenB64),
	}
	if pinned {
		parts = append(parts[:1], append([]string{"$env:FRP_RELEASE_TAG=" + powerShellQuote(releaseTag)}, parts[1:]...)...)
	}
	parts = append(parts, "powershell -ExecutionPolicy Bypass -Command "+powerShellQuote("iwr -UseBasicParsing "+url+" | iex"))
	return strings.Join(parts, "; ")
}

func shellCommand(platform, executable string, args ...string) string {
	quote := posixQuote
	prefix := ""
	if platform == "windows" {
		quote = powerShellQuote
		prefix = "& "
	}
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, quote(executable))
	for _, arg := range args {
		parts = append(parts, quote(arg))
	}
	return prefix + strings.Join(parts, " ")
}

func posixQuote(value string) string {
	if value == "" {
		return "''"
	}
	if isSafeShellWord(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func powerShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func isSafeShellWord(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '_', '@', '%', '+', '=', ':', ',', '.', '/', '-':
			continue
		default:
			return false
		}
	}
	return true
}

func defaultOpenTTY() (io.ReadCloser, error) {
	// Intentionally require stdout to remain attached to a terminal before using
	// /dev/tty or CONIN$ for prompts. Redirected-output wizard runs are treated
	// as unsupported operator flow and should fall back to plain stdin behavior.
	if !isCharDevice(os.Stdout) {
		return nil, errors.New("stdout is not a terminal")
	}
	if runtime.GOOS == "windows" {
		return os.OpenFile("CONIN$", os.O_RDONLY, 0)
	}
	return os.OpenFile("/dev/tty", os.O_RDONLY, 0)
}

func isCharDevice(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
