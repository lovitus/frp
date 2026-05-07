package gateway

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/sagernet/sing-shadowsocks"
	"github.com/sagernet/sing-shadowsocks/shadowaead"
	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

const GatewaySingSSUDPTimeout = 30 * time.Second

var GatewaySingSSMethods = append(append([]string{}, shadowaead.List...), shadowaead_2022.List...)

type noopSingSSHandler struct{}

func (noopSingSSHandler) NewConnection(context.Context, net.Conn, M.Metadata) error { return nil }

func (noopSingSSHandler) NewPacketConnection(context.Context, N.PacketConn, M.Metadata) error {
	return nil
}

func (noopSingSSHandler) NewError(context.Context, error) {}

func NewGatewaySingSSService(method, password string, handler shadowsocks.Handler) (shadowsocks.Service, error) {
	if handler == nil {
		handler = noopSingSSHandler{}
	}
	switch {
	case containsGatewaySingSSMethod(shadowaead.List, method):
		return shadowaead.NewService(method, nil, password, int64(GatewaySingSSUDPTimeout/time.Second), handler)
	case containsGatewaySingSSMethod(shadowaead_2022.List, method):
		return shadowaead_2022.NewServiceWithPassword(method, password, int64(GatewaySingSSUDPTimeout/time.Second), handler, nil)
	default:
		return nil, fmt.Errorf("shadowsocks: unsupported method %s", method)
	}
}

func ValidateGatewaySingSSMethod(protocol, method, password string) error {
	if _, err := NewGatewaySingSSService(method, password, noopSingSSHandler{}); err != nil {
		return fmt.Errorf("invalid ssMethod %q: %w", method, err)
	}
	return nil
}

func containsGatewaySingSSMethod(items []string, method string) bool {
	for _, item := range items {
		if item == method {
			return true
		}
	}
	return false
}

var _ E.Handler = noopSingSSHandler{}
