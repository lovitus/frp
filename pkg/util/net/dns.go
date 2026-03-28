// Copyright 2023 The frp Authors
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

package net

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func SetDefaultDNSAddress(dnsAddress string) {
	_ = SetDefaultDNSServers([]string{dnsAddress})
}

func SetDefaultDNSServers(servers []string) error {
	normalized := make([]string, 0, len(servers))
	for _, server := range servers {
		addr, err := NormalizeDNSServerAddress(server)
		if err != nil {
			return err
		}
		normalized = append(normalized, addr)
	}
	if len(normalized) == 0 {
		return fmt.Errorf("no dns servers configured")
	}

	// Change default dns server
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			if network == "" {
				network = "udp"
			}
			dialer := net.Dialer{Timeout: 5 * time.Second}
			var lastErr error
			for _, server := range normalized {
				conn, err := dialer.DialContext(ctx, network, server)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
	return nil
}

func NormalizeDNSServerAddress(server string) (string, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return "", fmt.Errorf("dns server is empty")
	}
	if host, port, err := net.SplitHostPort(server); err == nil {
		if host == "" || port == "" {
			return "", fmt.Errorf("dns server should contain both host and port")
		}
		return net.JoinHostPort(host, port), nil
	}
	if ip := net.ParseIP(server); ip != nil {
		return net.JoinHostPort(server, "53"), nil
	}
	if strings.Contains(server, ":") {
		return "", fmt.Errorf("ipv6 dns server with explicit port should use [addr]:port")
	}
	return net.JoinHostPort(server, "53"), nil
}

func ParseResolvConfServers(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var servers []string
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		addr, err := NormalizeDNSServerAddress(fields[1])
		if err != nil {
			continue
		}
		if addr == "0.0.0.0:53" || addr == "[::]:53" {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		servers = append(servers, addr)
	}
	return servers
}

func ParseResolvConfPaths(paths ...string) []string {
	seen := make(map[string]struct{})
	var servers []string
	for _, path := range paths {
		for _, server := range ParseResolvConfServers(path) {
			if _, ok := seen[server]; ok {
				continue
			}
			seen[server] = struct{}{}
			servers = append(servers, server)
		}
	}
	return servers
}
