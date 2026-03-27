package v1

import (
	"fmt"
	"slices"
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

func (c *ClientCommonConfig) IsMixEnabled() bool {
	return c.MixBindPort > 0 || c.MixToken != ""
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
