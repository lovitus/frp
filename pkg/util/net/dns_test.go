package net

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeDNSServerAddress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "ipv4 default port", input: "8.8.8.8", want: "8.8.8.8:53"},
		{name: "hostname default port", input: "dns.example.com", want: "dns.example.com:53"},
		{name: "ipv4 explicit port", input: "1.1.1.1:5353", want: "1.1.1.1:5353"},
		{name: "ipv6 default port", input: "2001:4860:4860::8888", want: "[2001:4860:4860::8888]:53"},
		{name: "ipv6 explicit port", input: "[2001:4860:4860::8844]:5353", want: "[2001:4860:4860::8844]:5353"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeDNSServerAddress(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseResolvConfServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	content := `
# comment
nameserver 8.8.8.8
nameserver 1.1.1.1:5353
nameserver 2001:4860:4860::8888
nameserver 0.0.0.0
nameserver 8.8.8.8
search example.com
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	got := ParseResolvConfServers(path)
	require.Equal(t, []string{
		"8.8.8.8:53",
		"1.1.1.1:5353",
		"[2001:4860:4860::8888]:53",
	}, got)
}

func TestParseResolvConfPathsDeduplicates(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "resolv1.conf")
	path2 := filepath.Join(dir, "resolv2.conf")
	require.NoError(t, os.WriteFile(path1, []byte("nameserver 8.8.8.8\n"), 0o644))
	require.NoError(t, os.WriteFile(path2, []byte("nameserver 8.8.8.8\nnameserver 1.1.1.1\n"), 0o644))

	got := ParseResolvConfPaths(path1, path2)
	require.Equal(t, []string{"8.8.8.8:53", "1.1.1.1:53"}, got)
}
