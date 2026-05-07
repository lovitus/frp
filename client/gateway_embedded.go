package client

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	gosocks5 "github.com/armon/go-socks5"
	"github.com/sagernet/sing-shadowsocks/shadowaead"
	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/uot"
	sscore "github.com/shadowsocks/go-shadowsocks2/core"
	"github.com/shadowsocks/go-shadowsocks2/socks"

	gatewaypkg "github.com/fatedier/frp/pkg/gateway"
	"github.com/fatedier/frp/pkg/msg"
)

const gatewayUDPBufSize = 64 * 1024

const (
	gatewaySingSSPacketHeadroom = shadowaead_2022.PacketNonceSize +
		16 + // packet header
		1 + // header type
		8 + // timestamp
		8 + // remote session id
		2 + // padding length
		shadowaead_2022.MaxPaddingLength +
		M.MaxSocksaddrLength
	gatewaySingSSPacketRearHeadroom = shadowaead.Overhead
)

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
	case gatewaypkg.TargetTypeSingSSProxy:
		switch cfg.Protocol {
		case "tcp":
			return newSingShadowsocksTCPService(cfg)
		case "udp":
			return newSingShadowsocksUDPService(cfg)
		default:
			return nil, errors.New("unsupported protocol for sing ss proxy")
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
	case gatewaypkg.TargetTypeSingSSProxy:
		return prev.SSMethod == next.SSMethod &&
			prev.SSPassword == next.SSPassword &&
			prev.UOTEnabled == next.UOTEnabled &&
			prev.UOTVersion == next.UOTVersion
	case gatewaypkg.TargetTypeSocks5Proxy:
		return prev.Socks5Auth == next.Socks5Auth &&
			prev.Socks5User == next.Socks5User &&
			prev.Socks5Pass == next.Socks5Pass
	default:
		return false
	}
}

type gatewaySingSSHandler struct {
	uotEnabled bool
	uotVersion int
}

func (h *gatewaySingSSHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	defer conn.Close()
	if h.uotEnabled && isGatewayUOTDestination(metadata.Destination, h.uotVersion) {
		packetConn := newGatewayMultiUDPConn(gatewaypkg.GatewaySingSSUDPTimeout)
		uotConn := uot.NewServerConn(packetConn, h.uotVersion)
		defer uotConn.Close()
		return relayGatewayTCPAndClose(conn, uotConn)
	}

	targetConn, err := net.Dial("tcp", metadata.Destination.String())
	if err != nil {
		return err
	}
	defer targetConn.Close()
	return relayGatewayTCP(conn, targetConn)
}

func (h *gatewaySingSSHandler) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	return relayGatewaySingSSUDPConn(ctx, conn)
}

func (h *gatewaySingSSHandler) NewError(context.Context, error) {}

func newSingShadowsocksTCPService(cfg msg.GatewayTunnelConfig) (gatewayEmbeddedService, error) {
	handler := &gatewaySingSSHandler{uotEnabled: cfg.UOTEnabled, uotVersion: cfg.UOTVersion}
	service, err := gatewaypkg.NewGatewaySingSSService(cfg.SSMethod, cfg.SSPassword, handler)
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
				if err := service.NewConnection(context.Background(), c, M.Metadata{
					Source: M.SocksaddrFromNet(c.RemoteAddr()),
				}); err != nil {
					service.NewError(context.Background(), err)
					_ = c.Close()
				}
			}(conn)
		}
	}()
	return svc, nil
}

func newSingShadowsocksUDPService(cfg msg.GatewayTunnelConfig) (gatewayEmbeddedService, error) {
	handler := &gatewaySingSSHandler{}
	service, err := gatewaypkg.NewGatewaySingSSService(cfg.SSMethod, cfg.SSPassword, handler)
	if err != nil {
		return nil, err
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	host, port := splitHostPort(pc.LocalAddr())
	svc := &gatewayUDPService{conn: pc, host: host, port: port}
	go runSingShadowsocksUDPServer(pc, service)
	return svc, nil
}

func runSingShadowsocksUDPServer(conn net.PacketConn, service interface {
	NewPacket(context.Context, N.PacketConn, *buf.Buffer, M.Metadata) error
	NewError(context.Context, error)
}) {
	packetConn := bufio.NewPacketConn(conn)
	for {
		buffer := buf.NewPacket()
		n, raddr, err := conn.ReadFrom(buffer.FreeBytes())
		if err != nil {
			buffer.Release()
			return
		}
		buffer.Truncate(n)
		if err := service.NewPacket(context.Background(), packetConn, buffer, M.Metadata{
			Source: M.SocksaddrFromNet(raddr),
		}); err != nil {
			buffer.Release()
			service.NewError(context.Background(), err)
		}
	}
}

func isGatewayUOTDestination(destination M.Socksaddr, version int) bool {
	switch version {
	case uot.LegacyVersion:
		return destination.Fqdn == uot.LegacyMagicAddress
	default:
		return destination.Fqdn == uot.MagicAddress
	}
}

type gatewayUDPPacket struct {
	data []byte
	addr net.Addr
}

type gatewayUDPUpstream struct {
	conn net.Conn
	addr net.Addr
}

type gatewayMultiUDPConn struct {
	timeout   time.Duration
	localAddr net.Addr

	mu        sync.Mutex
	sessions  map[string]*gatewayUDPUpstream
	responses chan gatewayUDPPacket
	closed    chan struct{}
	once      sync.Once
}

func newGatewayMultiUDPConn(timeout time.Duration) *gatewayMultiUDPConn {
	return &gatewayMultiUDPConn{
		timeout:   timeout,
		localAddr: &net.UDPAddr{IP: net.IPv4zero, Port: 0},
		sessions:  make(map[string]*gatewayUDPUpstream),
		responses: make(chan gatewayUDPPacket, 64),
		closed:    make(chan struct{}),
	}
}

func (c *gatewayMultiUDPConn) ReadFrom(p []byte) (int, net.Addr, error) {
	select {
	case packet := <-c.responses:
		return copy(p, packet.data), packet.addr, nil
	case <-c.closed:
		return 0, nil, net.ErrClosed
	}
}

func (c *gatewayMultiUDPConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if addr == nil {
		return 0, errors.New("missing udp destination")
	}
	session, err := c.session(addr)
	if err != nil {
		return 0, err
	}
	return session.conn.Write(p)
}

func (c *gatewayMultiUDPConn) Close() error {
	c.once.Do(func() {
		close(c.closed)
		c.mu.Lock()
		for key, session := range c.sessions {
			delete(c.sessions, key)
			_ = session.conn.Close()
		}
		c.mu.Unlock()
	})
	return nil
}

func (c *gatewayMultiUDPConn) LocalAddr() net.Addr                { return c.localAddr }
func (c *gatewayMultiUDPConn) SetDeadline(t time.Time) error      { return nil }
func (c *gatewayMultiUDPConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *gatewayMultiUDPConn) SetWriteDeadline(t time.Time) error { return nil }

func (c *gatewayMultiUDPConn) session(addr net.Addr) (*gatewayUDPUpstream, error) {
	key := addr.String()
	c.mu.Lock()
	if session := c.sessions[key]; session != nil {
		c.mu.Unlock()
		return session, nil
	}
	sessionConn, err := net.Dial("udp", addr.String())
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	session := &gatewayUDPUpstream{conn: sessionConn, addr: addr}
	c.sessions[key] = session
	c.mu.Unlock()
	go c.readSession(key, session)
	return session, nil
}

func (c *gatewayMultiUDPConn) readSession(key string, session *gatewayUDPUpstream) {
	defer func() {
		c.mu.Lock()
		if c.sessions[key] == session {
			delete(c.sessions, key)
		}
		c.mu.Unlock()
		_ = session.conn.Close()
	}()
	buffer := make([]byte, gatewayUDPBufSize)
	for {
		_ = session.conn.SetReadDeadline(time.Now().Add(c.timeout))
		n, err := session.conn.Read(buffer)
		if err != nil {
			return
		}
		packet := gatewayUDPPacket{
			data: append([]byte(nil), buffer[:n]...),
			addr: session.addr,
		}
		select {
		case c.responses <- packet:
		case <-c.closed:
			return
		}
	}
}

func relayGatewaySingSSUDPConn(ctx context.Context, conn N.PacketConn) error {
	defer conn.Close()
	udpConn := newGatewayMultiUDPConn(gatewaypkg.GatewaySingSSUDPTimeout)
	defer udpConn.Close()

	errCh := make(chan error, 2)
	go func() {
		for {
			buffer := buf.NewPacket()
			destination, err := conn.ReadPacket(buffer)
			if err != nil {
				buffer.Release()
				errCh <- err
				return
			}
			_, err = udpConn.WriteTo(buffer.Bytes(), destination)
			buffer.Release()
			if err != nil {
				errCh <- err
				return
			}
		}
	}()
	go func() {
		packet := make([]byte, gatewayUDPBufSize)
		for {
			n, addr, err := udpConn.ReadFrom(packet)
			if err != nil {
				errCh <- err
				return
			}
			buffer := buf.NewSize(gatewaySingSSPacketHeadroom + n + gatewaySingSSPacketRearHeadroom)
			buffer.Resize(gatewaySingSSPacketHeadroom, 0)
			_, err = buffer.Write(packet[:n])
			if err != nil {
				buffer.Release()
				errCh <- err
				return
			}
			if err := conn.WritePacket(buffer, M.SocksaddrFromNet(addr)); err != nil {
				buffer.Release()
				errCh <- err
				return
			}
			buffer.Release()
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
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

func relayGatewayTCPAndClose(left, right net.Conn) error {
	errCh := make(chan error, 2)
	copyAndClose := func(dst, src net.Conn) {
		_, err := io.Copy(dst, src)
		_ = dst.Close()
		_ = src.Close()
		errCh <- err
	}
	go copyAndClose(right, left)
	go copyAndClose(left, right)

	err := <-errCh
	err1 := <-errCh
	if isExpectedGatewayRelayClose(err) {
		err = nil
	}
	if isExpectedGatewayRelayClose(err1) {
		err1 = nil
	}
	if err != nil {
		return err
	}
	return err1
}

func isExpectedGatewayRelayClose(err error) bool {
	return err == nil ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, os.ErrDeadlineExceeded) ||
		errors.Is(err, io.ErrClosedPipe)
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
