package client

import (
	"errors"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	gosocks5 "github.com/armon/go-socks5"
	sscore "github.com/shadowsocks/go-shadowsocks2/core"
	"github.com/shadowsocks/go-shadowsocks2/socks"

	gatewaypkg "github.com/fatedier/frp/pkg/gateway"
	"github.com/fatedier/frp/pkg/msg"
)

const gatewayUDPBufSize = 64 * 1024

type gatewayEmbeddedService interface {
	Endpoint() (string, int)
	Close() error
}

type gatewayTCPService struct {
	listener net.Listener
	host     string
	port     int
}

func (s *gatewayTCPService) Endpoint() (string, int) {
	return s.host, s.port
}

func (s *gatewayTCPService) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

type gatewayUDPService struct {
	conn net.PacketConn
	host string
	port int
}

func (s *gatewayUDPService) Endpoint() (string, int) {
	return s.host, s.port
}

func (s *gatewayUDPService) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func newGatewayEmbeddedService(cfg msg.GatewayTunnelConfig) (gatewayEmbeddedService, error) {
	switch cfg.TargetType {
	case gatewaypkg.TargetTypeSSProxy:
		switch cfg.Protocol {
		case "tcp":
			return newShadowsocksTCPService(cfg)
		case "udp":
			return newShadowsocksUDPService(cfg)
		default:
			return nil, errors.New("unsupported protocol for ss proxy")
		}
	case gatewaypkg.TargetTypeSocks5Proxy:
		if cfg.Protocol != "tcp" {
			return nil, errors.New("socks5 proxy currently supports tcp only")
		}
		return newSocks5TCPService(cfg)
	default:
		return nil, errors.New("unsupported embedded gateway service")
	}
}

func canReuseGatewayEmbeddedService(prev, next msg.GatewayTunnelConfig) bool {
	if prev.TargetType != next.TargetType || prev.Protocol != next.Protocol {
		return false
	}
	switch next.TargetType {
	case gatewaypkg.TargetTypeSSProxy:
		return prev.SSMethod == next.SSMethod && prev.SSPassword == next.SSPassword
	case gatewaypkg.TargetTypeSocks5Proxy:
		return prev.Socks5Auth == next.Socks5Auth &&
			prev.Socks5User == next.Socks5User &&
			prev.Socks5Pass == next.Socks5Pass
	default:
		return false
	}
}

func newSocks5TCPService(cfg msg.GatewayTunnelConfig) (gatewayEmbeddedService, error) {
	conf := &gosocks5.Config{
		Logger: log.New(io.Discard, "", log.LstdFlags),
	}
	if cfg.Socks5Auth {
		conf.Credentials = gosocks5.StaticCredentials(map[string]string{
			cfg.Socks5User: cfg.Socks5Pass,
		})
	}
	server, err := gosocks5.New(conf)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	host, port := splitHostPort(ln.Addr())
	svc := &gatewayTCPService{listener: ln, host: host, port: port}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = server.ServeConn(c)
			}(conn)
		}
	}()
	return svc, nil
}

func newShadowsocksTCPService(cfg msg.GatewayTunnelConfig) (gatewayEmbeddedService, error) {
	ciph, err := sscore.PickCipher(cfg.SSMethod, nil, cfg.SSPassword)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	host, port := splitHostPort(ln.Addr())
	svc := &gatewayTCPService{listener: ln, host: host, port: port}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleShadowsocksTCPConn(conn, ciph)
		}
	}()
	return svc, nil
}

func handleShadowsocksTCPConn(conn net.Conn, ciph sscore.Cipher) {
	defer conn.Close()
	sc := ciph.StreamConn(conn)
	target, err := socks.ReadAddr(sc)
	if err != nil {
		return
	}
	rc, err := net.Dial("tcp", target.String())
	if err != nil {
		return
	}
	defer rc.Close()
	_ = relayGatewayTCP(sc, rc)
}

func relayGatewayTCP(left, right net.Conn) error {
	var err, err1 error
	var wg sync.WaitGroup
	const wait = 5 * time.Second

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err1 = io.Copy(right, left)
		_ = right.SetReadDeadline(time.Now().Add(wait))
	}()
	_, err = io.Copy(left, right)
	_ = left.SetReadDeadline(time.Now().Add(wait))
	wg.Wait()
	if err1 != nil && !errors.Is(err1, os.ErrDeadlineExceeded) {
		return err1
	}
	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	}
	return nil
}

type gatewayNATMap struct {
	sync.RWMutex
	entries map[string]net.PacketConn
	timeout time.Duration
}

func newGatewayNATMap(timeout time.Duration) *gatewayNATMap {
	return &gatewayNATMap{
		entries: make(map[string]net.PacketConn),
		timeout: timeout,
	}
}

func (m *gatewayNATMap) Get(key string) net.PacketConn {
	m.RLock()
	defer m.RUnlock()
	return m.entries[key]
}

func (m *gatewayNATMap) Set(key string, pc net.PacketConn) {
	m.Lock()
	defer m.Unlock()
	m.entries[key] = pc
}

func (m *gatewayNATMap) Del(key string) net.PacketConn {
	m.Lock()
	defer m.Unlock()
	pc := m.entries[key]
	delete(m.entries, key)
	return pc
}

func (m *gatewayNATMap) Add(peer net.Addr, dst, src net.PacketConn) {
	m.Set(peer.String(), src)
	go func() {
		_ = timedCopyGatewayUDP(dst, peer, src, m.timeout)
		if pc := m.Del(peer.String()); pc != nil {
			_ = pc.Close()
		}
	}()
}

func newShadowsocksUDPService(cfg msg.GatewayTunnelConfig) (gatewayEmbeddedService, error) {
	ciph, err := sscore.PickCipher(cfg.SSMethod, nil, cfg.SSPassword)
	if err != nil {
		return nil, err
	}
	packetCipher, ok := ciph.(sscore.PacketConnCipher)
	if !ok {
		return nil, errors.New("selected ssMethod does not support udp")
	}
	pc, err := sscore.ListenPacket("udp", "127.0.0.1:0", packetCipher)
	if err != nil {
		return nil, err
	}
	host, port := splitHostPort(pc.LocalAddr())
	svc := &gatewayUDPService{conn: pc, host: host, port: port}
	go runShadowsocksUDPServer(pc)
	return svc, nil
}

func runShadowsocksUDPServer(conn net.PacketConn) {
	nm := newGatewayNATMap(30 * time.Second)
	buf := make([]byte, gatewayUDPBufSize)
	for {
		n, raddr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		target := socks.SplitAddr(buf[:n])
		if target == nil {
			continue
		}
		targetAddr, err := net.ResolveUDPAddr("udp", target.String())
		if err != nil {
			continue
		}
		payload := append([]byte(nil), buf[len(target):n]...)
		pc := nm.Get(raddr.String())
		if pc == nil {
			pc, err = net.ListenPacket("udp", "")
			if err != nil {
				continue
			}
			nm.Add(raddr, conn, pc)
		}
		_, _ = pc.WriteTo(payload, targetAddr)
	}
}

func timedCopyGatewayUDP(dst net.PacketConn, peer net.Addr, src net.PacketConn, timeout time.Duration) error {
	buf := make([]byte, gatewayUDPBufSize)
	for {
		_ = src.SetReadDeadline(time.Now().Add(timeout))
		n, raddr, err := src.ReadFrom(buf)
		if err != nil {
			return err
		}
		srcAddr := socks.ParseAddr(raddr.String())
		if srcAddr == nil {
			continue
		}
		packet := make([]byte, len(srcAddr)+n)
		copy(packet, srcAddr)
		copy(packet[len(srcAddr):], buf[:n])
		_, err = dst.WriteTo(packet, peer)
		if err != nil {
			return err
		}
	}
}

func splitHostPort(addr net.Addr) (string, int) {
	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "127.0.0.1", 0
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}
