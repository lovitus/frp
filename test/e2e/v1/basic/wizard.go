package basic

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/fatedier/frp/test/e2e/framework"
)

var _ = ginkgo.Describe("[Feature: Wizard]", func() {
	f := framework.NewDefaultFramework()

	ginkgo.It("frps generates config, verifies, smokes, and prints bootstrap commands", func() {
		bindPort := f.AllocPort()
		mixBindPort := f.AllocPort()
		webPort := f.AllocPort()
		input := fmt.Sprintf("%d\n%d\nsecret\n\n%d\n\n", bindPort, mixBindPort, webPort)

		output, err := runWizardCommand(framework.TestContext.FRPServerPath, f.TempDirectory, input, "--wizard")
		framework.ExpectNoError(err, output)

		configPath := filepath.Join(f.TempDirectory, "frps.toml")
		content, err := os.ReadFile(configPath)
		framework.ExpectNoError(err)
		expected := fmt.Sprintf(`bindPort = %d
mixBindPort = %d
mixToken = "ss://chacha20-ietf-poly1305:secret,kcp://secret,ssh://frp:secret"

webServer.addr = "0.0.0.0"
webServer.port = %d
webServer.user = "admin"
webServer.password = "secret"
`, bindPort, mixBindPort, webPort)
		framework.ExpectEqual(string(content), expected)
		framework.ExpectContainSubstring(output, "Smoke start check passed.")
		framework.ExpectContainSubstring(output, "Unix/macOS/Linux local frpc wizard command")
		framework.ExpectContainSubstring(output, "Windows PowerShell local frpc wizard command")
		framework.ExpectContainSubstring(output, "--mix-token-b64")
		framework.ExpectContainSubstring(output, "Unix/macOS/Linux frpc quick-deploy command")
		framework.ExpectContainSubstring(output, "Windows PowerShell frpc quick-deploy command")
	})

	ginkgo.It("frpc standalone generates toml and accepts smoke warning or pass", func() {
		mixBindPort := f.AllocPort()
		input := fmt.Sprintf("%d\nsecret\n127.0.0.1\nedge-standalone\n", mixBindPort)

		output, err := runWizardCommand(framework.TestContext.FRPClientPath, f.TempDirectory, input, "--wizard")
		framework.ExpectNoError(err, output)

		configPath := filepath.Join(f.TempDirectory, "frpc.toml")
		content, err := os.ReadFile(configPath)
		framework.ExpectNoError(err)
		expected := fmt.Sprintf(`serverAddr = "127.0.0.1"
mixBindPort = %d
mixToken = "ss://chacha20-ietf-poly1305:secret,kcp://secret,ssh://frp:secret"
# mixFallbackHosts = "backup-a.example.com,backup-b.example.com:7002"

clientID = "edge-standalone"
allowGatewayTunnels = true
mixAllowGateway = true
loginFailExit = false
`, mixBindPort)
		framework.ExpectEqual(string(content), expected)
		framework.ExpectContainSubstring(output, "Smoke start check")
		framework.ExpectContainSubstring(output, "Local start command")
	})

	ginkgo.It("frpc preset with token b64 asks only serverAddr and clientID", func() {
		mixBindPort := f.AllocPort()
		token := "ss://chacha20-ietf-poly1305:secret,kcp://secret,ssh://frp:secret"
		tokenB64 := base64.StdEncoding.EncodeToString([]byte(token))

		output, err := runWizardCommand(framework.TestContext.FRPClientPath, f.TempDirectory, "127.0.0.1\nedge-preset\n",
			"--wizard",
			"--mix-bind-port", strconv.Itoa(mixBindPort),
			"--mix-token", "ignored",
			"--mix-token-b64", tokenB64,
		)
		framework.ExpectNoError(err, output)
		framework.ExpectTrue(!strings.Contains(output, "Connection password"), output)
		framework.ExpectTrue(!strings.Contains(output, "mixBindPort ["), output)

		content, err := os.ReadFile(filepath.Join(f.TempDirectory, "frpc.toml"))
		framework.ExpectNoError(err)
		framework.ExpectContainSubstring(string(content), fmt.Sprintf("mixBindPort = %d", mixBindPort))
		framework.ExpectContainSubstring(string(content), fmt.Sprintf("mixToken = %q", token))
		framework.ExpectContainSubstring(string(content), `clientID = "edge-preset"`)
	})

	ginkgo.It("refuses to overwrite existing config", func() {
		configPath := filepath.Join(f.TempDirectory, "frps.toml")
		framework.ExpectNoError(os.WriteFile(configPath, []byte("keep"), 0o600))

		output, err := runWizardCommand(framework.TestContext.FRPServerPath, f.TempDirectory, "", "--wizard")
		framework.ExpectError(err, output)
		content, readErr := os.ReadFile(configPath)
		framework.ExpectNoError(readErr)
		framework.ExpectEqual(string(content), "keep")
		framework.ExpectContainSubstring(output, "already exists")
	})

	ginkgo.It("rejects frpc wizard with config_dir", func() {
		configDir := filepath.Join(f.TempDirectory, "configs")
		framework.ExpectNoError(os.Mkdir(configDir, 0o700))

		output, err := runWizardCommand(framework.TestContext.FRPClientPath, f.TempDirectory, "", "--wizard", "--config_dir", configDir)
		framework.ExpectError(err, output)
		framework.ExpectContainSubstring(output, "incompatible with --config_dir")
	})

	ginkgo.It("version takes precedence over wizard", func() {
		output, err := runWizardCommand(framework.TestContext.FRPClientPath, f.TempDirectory, "", "--version", "--wizard")
		framework.ExpectNoError(err, output)
		framework.ExpectTrue(!strings.Contains(output, "Configure frpc"), output)
		_, statErr := os.Stat(filepath.Join(f.TempDirectory, "frpc.toml"))
		framework.ExpectTrue(os.IsNotExist(statErr), output)
	})
})

func runWizardCommand(binary string, cwd string, stdin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(stdin)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if ctx.Err() != nil {
		return output.String(), ctx.Err()
	}
	return output.String(), err
}
