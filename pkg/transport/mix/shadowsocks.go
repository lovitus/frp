package mix

import (
	"fmt"
	"net"

	sscore "github.com/shadowsocks/go-shadowsocks2/core"
	"github.com/shadowsocks/go-shadowsocks2/socks"
)

const ssTargetAddr = "mix.frp.invalid:0"

func NewShadowsocksCipher(method, password string) (sscore.Cipher, error) {
	return sscore.PickCipher(method, nil, password)
}

func WrapShadowsocksClient(conn net.Conn, method, password string) (net.Conn, error) {
	ciph, err := NewShadowsocksCipher(method, password)
	if err != nil {
		return nil, err
	}
	sc := ciph.StreamConn(conn)
	target := socks.ParseAddr(ssTargetAddr)
	if target == nil {
		return nil, fmt.Errorf("invalid shadowsocks target marker")
	}
	if _, err := sc.Write(target); err != nil {
		return nil, err
	}
	return sc, nil
}

func WrapShadowsocksServer(conn net.Conn, method, password string) (net.Conn, error) {
	ciph, err := NewShadowsocksCipher(method, password)
	if err != nil {
		return nil, err
	}
	sc := ciph.StreamConn(conn)
	addr, err := socks.ReadAddr(sc)
	if err != nil {
		return nil, err
	}
	if addr.String() != ssTargetAddr {
		return nil, fmt.Errorf("unexpected shadowsocks target %q", addr.String())
	}
	return sc, nil
}
