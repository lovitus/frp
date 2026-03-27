package system

import (
	"os"
	"path/filepath"
	"strings"

	netpkg "github.com/fatedier/frp/pkg/util/net"
)

var compatibilityFallbackDNSServers = []string{"8.8.8.8:53", "1.1.1.1:53"}

func fixRestrictedDNSResolver() {
	if !shouldInstallCompatibilityDNSResolver() {
		return
	}
	servers := discoverCompatibilityDNSServers()
	if len(servers) == 0 {
		servers = compatibilityFallbackDNSServers
	}
	_ = netpkg.SetDefaultDNSServers(servers)
}

func shouldInstallCompatibilityDNSResolver() bool {
	return isTermuxLikeRuntime() || isAndroidLikeRuntime()
}

func isTermuxLikeRuntime() bool {
	if os.Getenv("TERMUX_VERSION") != "" {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(os.Getenv("PREFIX")), "/data/data/com.termux")
}

func isAndroidLikeRuntime() bool {
	return os.Getenv("ANDROID_ROOT") != "" || os.Getenv("ANDROID_DATA") != ""
}

func discoverCompatibilityDNSServers() []string {
	paths := []string{"/etc/resolv.conf"}
	if prefix := strings.TrimSpace(os.Getenv("PREFIX")); prefix != "" {
		paths = append(paths, filepath.Join(prefix, "etc", "resolv.conf"))
	}
	return netpkg.ParseResolvConfPaths(paths...)
}
