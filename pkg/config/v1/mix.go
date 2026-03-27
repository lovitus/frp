package v1

import (
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
)

const (
	MixProtocolKCP  = "kcp"
	MixProtocolQUIC = "quic"
	MixProtocolSS   = "ss"
	MixProtocolWSS  = "wss"
	MixProtocolSSH  = "ssh"
	MixProtocolTCP  = "tcp"
)

var SupportedMixProtocols = []string{
	MixProtocolKCP,
	MixProtocolQUIC,
	MixProtocolSS,
	MixProtocolWSS,
	MixProtocolSSH,
	MixProtocolTCP,
}

type MixProtocolConfig struct {
	Protocol string `json:"protocol,omitempty"`
	Method   string `json:"method,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type MixEndpointConfig struct {
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
}

func (c MixEndpointConfig) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func (c *ClientCommonConfig) IsMixEnabled() bool {
	return c.MixBindPort > 0 || c.MixToken != "" || c.MixFallbackHosts != ""
}

func (c *ServerConfig) IsMixEnabled() bool {
	return c.MixBindPort > 0 || c.MixToken != ""
}

func ParseMixToken(token string) ([]MixProtocolConfig, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("mix token is empty")
	}

	parts := strings.Split(token, ",")
	out := make([]MixProtocolConfig, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, raw := range parts {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil, fmt.Errorf("mix token contains empty entry")
		}
		scheme, payload, ok := strings.Cut(raw, "://")
		if !ok {
			return nil, fmt.Errorf("mix token %q should contain ://", raw)
		}
		scheme = strings.ToLower(strings.TrimSpace(scheme))
		payload = strings.TrimSpace(payload)
		if !slices.Contains(SupportedMixProtocols, scheme) {
			return nil, fmt.Errorf("unsupported mix protocol %q", scheme)
		}
		if _, ok := seen[scheme]; ok {
			return nil, fmt.Errorf("duplicate mix protocol %q is not allowed", scheme)
		}
		seen[scheme] = struct{}{}

		entry := MixProtocolConfig{Protocol: scheme}
		switch scheme {
		case MixProtocolKCP, MixProtocolQUIC, MixProtocolWSS, MixProtocolTCP:
			if payload == "" {
				return nil, fmt.Errorf("%s token should be %s://PASSWORD", scheme, scheme)
			}
			entry.Password = payload
		case MixProtocolSSH:
			username, password, ok := strings.Cut(payload, ":")
			if !ok || username == "" || password == "" {
				return nil, fmt.Errorf("ssh token should be ssh://USERNAME:PASSWORD")
			}
			entry.Username = username
			entry.Password = password
		case MixProtocolSS:
			method, password, ok := strings.Cut(payload, ":")
			if !ok || method == "" || password == "" {
				return nil, fmt.Errorf("ss token should be ss://METHOD:PASSWORD")
			}
			entry.Method = method
			entry.Password = password
		}
		out = append(out, entry)
	}
	return out, nil
}

func BuildMixEndpoints(serverAddr string, mixBindPort int, fallbackHosts string) ([]MixEndpointConfig, error) {
	serverAddr = strings.TrimSpace(serverAddr)
	if serverAddr == "" {
		return nil, fmt.Errorf("server address is empty")
	}
	if mixBindPort <= 0 {
		return nil, fmt.Errorf("mix bind port should be greater than 0")
	}

	out := []MixEndpointConfig{{
		Host: serverAddr,
		Port: mixBindPort,
	}}
	seen := map[string]struct{}{
		out[0].Address(): {},
	}

	fallbacks, err := ParseMixFallbackHosts(fallbackHosts, mixBindPort)
	if err != nil {
		return nil, err
	}
	for _, endpoint := range fallbacks {
		key := endpoint.Address()
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate mix endpoint %q is not allowed", key)
		}
		seen[key] = struct{}{}
		out = append(out, endpoint)
	}
	return out, nil
}

func ParseMixFallbackHosts(value string, defaultPort int) ([]MixEndpointConfig, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if defaultPort <= 0 {
		return nil, fmt.Errorf("default mix port should be greater than 0")
	}

	parts := strings.Split(value, ",")
	out := make([]MixEndpointConfig, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, raw := range parts {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil, fmt.Errorf("mix fallback hosts contains empty entry")
		}
		endpoint, err := parseMixEndpoint(raw, defaultPort)
		if err != nil {
			return nil, fmt.Errorf("invalid mix fallback host %q: %w", raw, err)
		}
		key := endpoint.Address()
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate mix fallback host %q is not allowed", raw)
		}
		seen[key] = struct{}{}
		out = append(out, endpoint)
	}
	return out, nil
}

func parseMixEndpoint(value string, defaultPort int) (MixEndpointConfig, error) {
	if strings.HasPrefix(value, "[") {
		if host, port, err := net.SplitHostPort(value); err == nil {
			portNum, err := strconv.Atoi(port)
			if err != nil {
				return MixEndpointConfig{}, fmt.Errorf("invalid port %q", port)
			}
			return MixEndpointConfig{Host: host, Port: portNum}, nil
		}
		if strings.HasSuffix(value, "]") {
			host := strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
			if host == "" {
				return MixEndpointConfig{}, fmt.Errorf("host is empty")
			}
			return MixEndpointConfig{Host: host, Port: defaultPort}, nil
		}
		return MixEndpointConfig{}, fmt.Errorf("ipv6 host with explicit port should use [addr]:port")
	}

	colonCount := strings.Count(value, ":")
	switch {
	case colonCount == 0:
		if value == "" {
			return MixEndpointConfig{}, fmt.Errorf("host is empty")
		}
		return MixEndpointConfig{Host: value, Port: defaultPort}, nil
	case colonCount == 1:
		host, port, err := net.SplitHostPort(value)
		if err != nil {
			return MixEndpointConfig{}, err
		}
		if host == "" {
			return MixEndpointConfig{}, fmt.Errorf("host is empty")
		}
		portNum, err := strconv.Atoi(port)
		if err != nil {
			return MixEndpointConfig{}, fmt.Errorf("invalid port %q", port)
		}
		return MixEndpointConfig{Host: host, Port: portNum}, nil
	default:
		if ip := net.ParseIP(value); ip != nil {
			return MixEndpointConfig{Host: value, Port: defaultPort}, nil
		}
		return MixEndpointConfig{}, fmt.Errorf("ipv6 host with explicit port should use [addr]:port")
	}
}
