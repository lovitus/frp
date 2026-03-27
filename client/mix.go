package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	libnet "github.com/fatedier/golib/net"
	fmux "github.com/hashicorp/yamux"
	quic "github.com/quic-go/quic-go"
	"github.com/samber/lo"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/transport"
	tmix "github.com/fatedier/frp/pkg/transport/mix"
	"github.com/fatedier/frp/pkg/util/log"
	netpkg "github.com/fatedier/frp/pkg/util/net"
	"github.com/fatedier/frp/pkg/util/xlog"
)

var (
	mixFallbackDelay     = 10 * time.Second
	mixFailbackInterval  = 60 * time.Second
	mixFallbackThreshold = 3
)

type mixProbeCandidate struct {
	Index    int
	Protocol v1.MixProtocolConfig
}

type MixConnectorManager struct {
	mu sync.Mutex

	protocols []v1.MixProtocolConfig

	activeIndex    int
	failCount      int
	lastFailure    time.Time
	lastSwitchTime time.Time
	probing        bool
	switching      bool
	switchReason   string
}

func NewMixConnectorManager(cfg *v1.ClientCommonConfig) (*MixConnectorManager, error) {
	protocols, err := v1.ParseMixToken(cfg.MixToken)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		names = append(names, protocol.Protocol)
	}
	log.Infof("mix init, bind port [%d], protocols %v", cfg.MixBindPort, names)
	return &MixConnectorManager{
		protocols: protocols,
	}, nil
}

func (m *MixConnectorManager) NewConnector(ctx context.Context, cfg *v1.ClientCommonConfig) Connector {
	return &mixConnectorImpl{
		ctx:     ctx,
		cfg:     cfg,
		manager: m,
	}
}

func (m *MixConnectorManager) SelectedProtocol() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.protocols) == 0 {
		return ""
	}
	return m.protocols[m.activeIndex].Protocol
}

func (m *MixConnectorManager) CurrentActiveIndex() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeIndex
}

func (m *MixConnectorManager) prepareDial() (int, v1.MixProtocolConfig, time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.protocols[m.activeIndex]
	waitFor := time.Duration(0)
	if !m.lastFailure.IsZero() {
		next := m.lastFailure.Add(mixFallbackDelay)
		if d := time.Until(next); d > 0 {
			waitFor = d
		}
	}
	return m.activeIndex, entry, waitFor
}

func (m *MixConnectorManager) recordDialSuccess(index int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	reason := ""
	if m.switching {
		reason = m.switchReason
	}
	if m.activeIndex != index {
		m.activeIndex = index
		m.lastSwitchTime = time.Now()
	}
	m.failCount = 0
	m.lastFailure = time.Time{}
	m.switching = false
	m.switchReason = ""
	return reason
}

func (m *MixConnectorManager) recordDialFailure(index int) (int, *v1.MixProtocolConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index != m.activeIndex {
		return m.failCount, nil
	}
	m.failCount++
	m.lastFailure = time.Now()
	if m.failCount < mixFallbackThreshold || index+1 >= len(m.protocols) {
		return m.failCount, nil
	}

	m.activeIndex = index + 1
	m.failCount = 0
	m.lastFailure = time.Time{}
	m.lastSwitchTime = time.Now()
	m.switching = true
	m.switchReason = "fallback"
	entry := m.protocols[m.activeIndex]
	return mixFallbackThreshold, &entry
}

func (m *MixConnectorManager) nextFailbackCandidates() (int, []mixProbeCandidate, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeIndex <= 0 || m.probing || m.switching {
		return 0, nil, false
	}
	if !m.lastSwitchTime.IsZero() && time.Since(m.lastSwitchTime) < mixFailbackInterval {
		return 0, nil, false
	}
	m.probing = true
	base := m.activeIndex
	out := make([]mixProbeCandidate, 0, base)
	for i := range base {
		out = append(out, mixProbeCandidate{
			Index:    i,
			Protocol: m.protocols[i],
		})
	}
	return base, out, true
}

func (m *MixConnectorManager) finishProbe() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probing = false
}

func (m *MixConnectorManager) switchToPreferred(baseIndex, targetIndex int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	defer func() {
		m.probing = false
	}()
	if m.activeIndex != baseIndex || m.switching {
		return false
	}
	if targetIndex >= m.activeIndex {
		return false
	}
	m.activeIndex = targetIndex
	m.failCount = 0
	m.lastFailure = time.Time{}
	m.lastSwitchTime = time.Now()
	m.switching = true
	m.switchReason = "failback"
	return true
}

// SetMixTimingForTesting overrides mix retry timing defaults and returns a restore function.
// It is intended for automated tests so fallback/failback behavior can be verified quickly.
func SetMixTimingForTesting(fallbackDelay, failbackInterval time.Duration, fallbackThreshold int) func() {
	prevFallbackDelay := mixFallbackDelay
	prevFailbackInterval := mixFailbackInterval
	prevFallbackThreshold := mixFallbackThreshold

	mixFallbackDelay = fallbackDelay
	mixFailbackInterval = failbackInterval
	mixFallbackThreshold = fallbackThreshold

	return func() {
		mixFallbackDelay = prevFallbackDelay
		mixFailbackInterval = prevFailbackInterval
		mixFallbackThreshold = prevFallbackThreshold
	}
}

type mixConnectorImpl struct {
	ctx     context.Context
	cfg     *v1.ClientCommonConfig
	manager *MixConnectorManager

	inner            Connector
	selectedProtocol string
	activeIndex      int
	resultReported   bool
}

func (c *mixConnectorImpl) Open() error {
	xl := xlog.FromContextSafe(c.ctx)
	index, protocol, waitFor := c.manager.prepareDial()
	if waitFor > 0 {
		xl.Infof("mix protocol retry waiting for %s before dialing [%s]", waitFor.Round(time.Second), protocol.Protocol)
		timer := time.NewTimer(waitFor)
		defer timer.Stop()
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-timer.C:
		}
	}

	xl.Infof("mix protocol dial start, target protocol [%s], active index [%d]", protocol.Protocol, index)
	inner, err := newMixProtocolConnector(c.ctx, c.cfg, protocol)
	if err != nil {
		c.activeIndex = index
		c.selectedProtocol = protocol.Protocol
		c.ReportLoginFailure(err)
		return err
	}
	if err := inner.Open(); err != nil {
		c.activeIndex = index
		c.selectedProtocol = protocol.Protocol
		c.ReportLoginFailure(err)
		return err
	}
	c.inner = inner
	c.activeIndex = index
	c.selectedProtocol = protocol.Protocol
	return nil
}

func (c *mixConnectorImpl) Connect() (net.Conn, error) {
	if c.inner == nil {
		return nil, fmt.Errorf("mix connector is not opened")
	}
	conn, err := c.inner.Connect()
	if err != nil {
		c.ReportLoginFailure(err)
		return nil, err
	}
	return conn, nil
}

func (c *mixConnectorImpl) Close() error {
	if c.inner != nil {
		return c.inner.Close()
	}
	return nil
}

func (c *mixConnectorImpl) SelectedProtocol() string {
	return c.selectedProtocol
}

func (c *mixConnectorImpl) ReportLoginSuccess() {
	if c.resultReported {
		return
	}
	c.resultReported = true
	reason := c.manager.recordDialSuccess(c.activeIndex)
	xl := xlog.FromContextSafe(c.ctx)
	switch reason {
	case "fallback":
		xl.Infof("mix fallback success, active protocol [%s]", c.selectedProtocol)
	case "failback":
		xl.Infof("mix failback switch success, active protocol [%s]", c.selectedProtocol)
	}
}

func (c *mixConnectorImpl) ReportLoginFailure(err error) {
	if c.resultReported {
		return
	}
	c.resultReported = true
	xl := xlog.FromContextSafe(c.ctx)
	failCount, fallback := c.manager.recordDialFailure(c.activeIndex)
	xl.Warnf("mix protocol dial fail, protocol [%s], fail count [%d], err: %v", c.selectedProtocol, failCount, err)
	if fallback != nil {
		xl.Warnf("mix fallback start, switch to protocol [%s]", fallback.Protocol)
	}
}

func buildMixClientConfig(cfg *v1.ClientCommonConfig, protocol v1.MixProtocolConfig) (*v1.ClientCommonConfig, error) {
	cloned := *cfg
	cloned.ServerPort = cfg.MixBindPort
	cloned.Transport.TLS.Enable = lo.ToPtr(false)
	switch protocol.Protocol {
	case v1.MixProtocolTCP, v1.MixProtocolKCP, v1.MixProtocolQUIC, v1.MixProtocolWSS, v1.MixProtocolSS, v1.MixProtocolSSH:
		cloned.Transport.Protocol = protocol.Protocol
		if protocol.Protocol == v1.MixProtocolWSS {
			cloned.Transport.TLS.Enable = lo.ToPtr(true)
		}
	default:
		return nil, fmt.Errorf("mix protocol %q is not implemented by the current client transport stack", protocol.Protocol)
	}
	return &cloned, nil
}

func probeMixProtocol(ctx context.Context, cfg *v1.ClientCommonConfig, protocol v1.MixProtocolConfig) error {
	connector, err := newMixProtocolConnector(ctx, cfg, protocol)
	if err != nil {
		return err
	}
	if err := connector.Open(); err != nil {
		return err
	}
	defer connector.Close()
	conn, err := connector.Connect()
	if err != nil {
		return err
	}
	return conn.Close()
}

func newMixProtocolConnector(ctx context.Context, cfg *v1.ClientCommonConfig, protocol v1.MixProtocolConfig) (Connector, error) {
	switch protocol.Protocol {
	case v1.MixProtocolTCP, v1.MixProtocolKCP, v1.MixProtocolWSS:
		return newMixTokenConnector(ctx, cfg, protocol), nil
	case v1.MixProtocolQUIC:
		return newMixQUICConnector(ctx, cfg, protocol), nil
	case v1.MixProtocolSS:
		return newMixSSConnector(ctx, cfg, protocol), nil
	case v1.MixProtocolSSH:
		return newMixSSHConnector(ctx, cfg, protocol), nil
	default:
		return nil, fmt.Errorf("unsupported mix protocol %q", protocol.Protocol)
	}
}

type mixReusableStreamConnector struct {
	ctx      context.Context
	cfg      *v1.ClientCommonConfig
	protocol v1.MixProtocolConfig
	dialFn   func() (net.Conn, error)

	baseConn   net.Conn
	muxSession *fmux.Session
	closeOnce  sync.Once
}

func (c *mixReusableStreamConnector) Open() error {
	if !lo.FromPtr(c.cfg.Transport.TCPMux) {
		return nil
	}
	conn, err := c.dialFn()
	if err != nil {
		return err
	}
	cfg := fmux.DefaultConfig()
	cfg.KeepAliveInterval = time.Duration(c.cfg.Transport.TCPMuxKeepaliveInterval) * time.Second
	cfg.LogOutput = xlog.NewTraceWriter(xlog.FromContextSafe(c.ctx))
	cfg.MaxStreamWindowSize = 6 * 1024 * 1024
	session, err := fmux.Client(conn, cfg)
	if err != nil {
		_ = conn.Close()
		return err
	}
	c.baseConn = conn
	c.muxSession = session
	return nil
}

func (c *mixReusableStreamConnector) Connect() (net.Conn, error) {
	if c.muxSession != nil {
		return c.muxSession.OpenStream()
	}
	return c.dialFn()
}

func (c *mixReusableStreamConnector) Close() error {
	c.closeOnce.Do(func() {
		if c.muxSession != nil {
			_ = c.muxSession.Close()
		}
		if c.baseConn != nil {
			_ = c.baseConn.Close()
		}
	})
	return nil
}

func newMixTokenConnector(ctx context.Context, cfg *v1.ClientCommonConfig, protocol v1.MixProtocolConfig) Connector {
	return &mixReusableStreamConnector{
		ctx:      ctx,
		cfg:      cfg,
		protocol: protocol,
		dialFn: func() (net.Conn, error) {
			conn, err := dialMixBaseConn(ctx, cfg, protocol)
			if err != nil {
				return nil, err
			}
			if err := tmix.WriteToken(conn, protocol.Protocol, protocol.Password); err != nil {
				_ = conn.Close()
				return nil, err
			}
			if err := tmix.ReadTokenAck(conn); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return conn, nil
		},
	}
}

type mixQUICConnector struct {
	ctx      context.Context
	cfg      *v1.ClientCommonConfig
	protocol v1.MixProtocolConfig

	quicConn  *quic.Conn
	closeOnce sync.Once
}

func newMixQUICConnector(ctx context.Context, cfg *v1.ClientCommonConfig, protocol v1.MixProtocolConfig) Connector {
	return &mixQUICConnector{
		ctx:      ctx,
		cfg:      cfg,
		protocol: protocol,
	}
}

func (c *mixQUICConnector) Open() error {
	tlsConfig, err := buildMixQUICClientTLSConfig(c.cfg)
	if err != nil {
		return err
	}
	dialCtx, cancel := context.WithTimeout(
		c.ctx,
		time.Duration(c.cfg.Transport.DialServerTimeout)*time.Second,
	)
	defer cancel()
	conn, err := quic.DialAddr(
		dialCtx,
		net.JoinHostPort(c.cfg.ServerAddr, strconv.Itoa(c.cfg.MixBindPort)),
		tlsConfig,
		&quic.Config{
			MaxIdleTimeout:     time.Duration(c.cfg.Transport.QUIC.MaxIdleTimeout) * time.Second,
			MaxIncomingStreams: int64(c.cfg.Transport.QUIC.MaxIncomingStreams),
			KeepAlivePeriod:    time.Duration(c.cfg.Transport.QUIC.KeepalivePeriod) * time.Second,
		},
	)
	if err != nil {
		return err
	}
	c.quicConn = conn
	return nil
}

func (c *mixQUICConnector) Connect() (net.Conn, error) {
	if c.quicConn == nil {
		return nil, fmt.Errorf("quic connector is not opened")
	}
	stream, err := c.quicConn.OpenStreamSync(context.Background())
	if err != nil {
		return nil, err
	}
	conn := netpkg.QuicStreamToNetConn(stream, c.quicConn)
	if err := tmix.WriteToken(conn, c.protocol.Protocol, c.protocol.Password); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := tmix.ReadTokenAck(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (c *mixQUICConnector) Close() error {
	c.closeOnce.Do(func() {
		if c.quicConn != nil {
			_ = c.quicConn.CloseWithError(0, "")
		}
	})
	return nil
}

func newMixSSConnector(ctx context.Context, cfg *v1.ClientCommonConfig, protocol v1.MixProtocolConfig) Connector {
	return &mixReusableStreamConnector{
		ctx:      ctx,
		cfg:      cfg,
		protocol: protocol,
		dialFn: func() (net.Conn, error) {
			conn, err := dialMixTCPConn(ctx, cfg)
			if err != nil {
				return nil, err
			}
			ssConn, err := tmix.WrapShadowsocksClient(conn, protocol.Method, protocol.Password)
			if err != nil {
				_ = conn.Close()
				return nil, err
			}
			return ssConn, nil
		},
	}
}

type mixSSHConnector struct {
	ctx      context.Context
	cfg      *v1.ClientCommonConfig
	protocol v1.MixProtocolConfig

	sshClient  *sshClientWrapper
	muxSession *fmux.Session
	baseConn   net.Conn
	closeOnce  sync.Once
}

type sshClientWrapper struct {
	client  *ssh.Client
	rawConn net.Conn
}

func (w *sshClientWrapper) Close() error {
	if w.client != nil {
		_ = w.client.Close()
	}
	if w.rawConn != nil {
		return w.rawConn.Close()
	}
	return nil
}

func newMixSSHConnector(ctx context.Context, cfg *v1.ClientCommonConfig, protocol v1.MixProtocolConfig) Connector {
	return &mixSSHConnector{
		ctx:      ctx,
		cfg:      cfg,
		protocol: protocol,
	}
}

func (c *mixSSHConnector) Open() error {
	timeout := time.Duration(c.cfg.Transport.DialServerTimeout) * time.Second
	client, rawConn, err := tmix.DialSSH(
		c.ctx,
		net.JoinHostPort(c.cfg.ServerAddr, strconv.Itoa(c.cfg.MixBindPort)),
		c.protocol.Username,
		c.protocol.Password,
		timeout,
	)
	if err != nil {
		return err
	}
	c.sshClient = &sshClientWrapper{client: client, rawConn: rawConn}
	if !lo.FromPtr(c.cfg.Transport.TCPMux) {
		return nil
	}
	conn, err := tmix.OpenSSHChannel(client)
	if err != nil {
		_ = c.sshClient.Close()
		return err
	}
	fmuxCfg := fmux.DefaultConfig()
	fmuxCfg.KeepAliveInterval = time.Duration(c.cfg.Transport.TCPMuxKeepaliveInterval) * time.Second
	fmuxCfg.LogOutput = xlog.NewTraceWriter(xlog.FromContextSafe(c.ctx))
	fmuxCfg.MaxStreamWindowSize = 6 * 1024 * 1024
	session, err := fmux.Client(conn, fmuxCfg)
	if err != nil {
		_ = conn.Close()
		_ = c.sshClient.Close()
		return err
	}
	c.baseConn = conn
	c.muxSession = session
	return nil
}

func (c *mixSSHConnector) Connect() (net.Conn, error) {
	if c.muxSession != nil {
		return c.muxSession.OpenStream()
	}
	if c.sshClient == nil {
		return nil, fmt.Errorf("ssh connector is not opened")
	}
	return tmix.OpenSSHChannel(c.sshClient.client)
}

func (c *mixSSHConnector) Close() error {
	c.closeOnce.Do(func() {
		if c.muxSession != nil {
			_ = c.muxSession.Close()
		}
		if c.baseConn != nil {
			_ = c.baseConn.Close()
		}
		if c.sshClient != nil {
			_ = c.sshClient.Close()
		}
	})
	return nil
}

func dialMixTCPConn(ctx context.Context, cfg *v1.ClientCommonConfig) (net.Conn, error) {
	return dialMixNetworkConn(ctx, cfg, "tcp")
}

func dialMixKCPConn(ctx context.Context, cfg *v1.ClientCommonConfig) (net.Conn, error) {
	return dialMixNetworkConn(ctx, cfg, "kcp")
}

func dialMixNetworkConn(ctx context.Context, cfg *v1.ClientCommonConfig, protocol string) (net.Conn, error) {
	proxyType, addr, auth, err := libnet.ParseProxyURL(cfg.Transport.ProxyURL)
	if err != nil {
		return nil, err
	}
	dialOptions := []libnet.DialOption{}
	if cfg.Transport.ConnectServerLocalIP != "" && protocol == "tcp" {
		dialOptions = append(dialOptions, libnet.WithLocalAddr(cfg.Transport.ConnectServerLocalIP))
	}
	dialOptions = append(dialOptions,
		libnet.WithProtocol(protocol),
		libnet.WithTimeout(time.Duration(cfg.Transport.DialServerTimeout)*time.Second),
		libnet.WithKeepAlive(time.Duration(cfg.Transport.DialServerKeepAlive)*time.Second),
		libnet.WithProxy(proxyType, addr),
		libnet.WithProxyAuth(auth),
	)
	return libnet.DialContext(
		ctx,
		net.JoinHostPort(cfg.ServerAddr, strconv.Itoa(cfg.MixBindPort)),
		dialOptions...,
	)
}

func dialMixBaseConn(ctx context.Context, cfg *v1.ClientCommonConfig, protocol v1.MixProtocolConfig) (net.Conn, error) {
	switch protocol.Protocol {
	case v1.MixProtocolTCP:
		return dialMixTCPConn(ctx, cfg)
	case v1.MixProtocolKCP:
		return dialMixKCPConn(ctx, cfg)
	case v1.MixProtocolWSS:
		rawConn, err := dialMixTCPConn(ctx, cfg)
		if err != nil {
			return nil, err
		}
		tlsCfg, err := buildMixClientTLSConfig(cfg)
		if err != nil {
			_ = rawConn.Close()
			return nil, err
		}
		tlsConn := tls.Client(rawConn, tlsCfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = rawConn.Close()
			return nil, err
		}
		host := net.JoinHostPort(tlsCfg.ServerName, strconv.Itoa(cfg.MixBindPort))
		target := "wss://" + host + netpkg.FrpWebsocketPath
		cfgWS, err := websocket.NewConfig(target, "http://"+host)
		if err != nil {
			_ = tlsConn.Close()
			return nil, err
		}
		wsConn, err := websocket.NewClient(cfgWS, tlsConn)
		if err != nil {
			_ = tlsConn.Close()
			return nil, err
		}
		return wsConn, nil
	default:
		return nil, fmt.Errorf("unsupported base mix protocol %q", protocol.Protocol)
	}
}

func buildMixClientTLSConfig(cfg *v1.ClientCommonConfig) (*tls.Config, error) {
	serverName := cfg.Transport.TLS.ServerName
	if serverName == "" {
		serverName = cfg.ServerAddr
	}
	return transport.NewClientTLSConfig(
		cfg.Transport.TLS.CertFile,
		cfg.Transport.TLS.KeyFile,
		cfg.Transport.TLS.TrustedCaFile,
		serverName,
	)
}

func buildMixQUICClientTLSConfig(cfg *v1.ClientCommonConfig) (*tls.Config, error) {
	serverName := cfg.Transport.TLS.ServerName
	if serverName == "" {
		serverName = cfg.ServerAddr
	}
	tlsConfig, err := transport.NewClientTLSConfig(
		cfg.Transport.TLS.CertFile,
		cfg.Transport.TLS.KeyFile,
		cfg.Transport.TLS.TrustedCaFile,
		serverName,
	)
	if err != nil {
		return nil, err
	}
	tlsConfig.NextProtos = []string{"frp"}
	return tlsConfig, nil
}
