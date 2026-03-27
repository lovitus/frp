package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsTermuxLikeRuntime(t *testing.T) {
	t.Setenv("TERMUX_VERSION", "0.118.0")
	require.True(t, isTermuxLikeRuntime())
}

func TestIsTermuxLikeRuntimeFromPrefix(t *testing.T) {
	t.Setenv("TERMUX_VERSION", "")
	t.Setenv("PREFIX", "/data/data/com.termux/files/usr")
	require.True(t, isTermuxLikeRuntime())
}

func TestIsAndroidLikeRuntime(t *testing.T) {
	t.Setenv("ANDROID_ROOT", "/system")
	t.Setenv("ANDROID_DATA", "")
	require.True(t, isAndroidLikeRuntime())
}

func TestDiscoverCompatibilityDNSServersIncludesPrefixResolvConf(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PREFIX", dir)
	resolvPath := filepath.Join(dir, "etc", "resolv.conf")
	require.NoError(t, os.MkdirAll(filepath.Dir(resolvPath), 0o755))
	require.NoError(t, os.WriteFile(resolvPath, []byte("nameserver 9.9.9.9\n"), 0o644))

	got := discoverCompatibilityDNSServers()
	require.Contains(t, got, "9.9.9.9:53")
}

func TestShouldInstallCompatibilityDNSResolver(t *testing.T) {
	t.Setenv("TERMUX_VERSION", "")
	t.Setenv("PREFIX", "")
	t.Setenv("ANDROID_ROOT", "")
	t.Setenv("ANDROID_DATA", "")
	require.False(t, shouldInstallCompatibilityDNSResolver())

	t.Setenv("ANDROID_DATA", "/data")
	require.True(t, shouldInstallCompatibilityDNSResolver())
}
