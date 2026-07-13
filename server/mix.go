package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	gnet "github.com/fatedier/golib/net"
	quic "github.com/quic-go/quic-go"
	"github.com/samber/lo"
	kcp "github.com/xtaci/kcp-go/v5"
	"golang.org/x/crypto/ssh"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/transport"
	tmix "github.com/fatedier/frp/pkg/transport/mix"
	"github.com/fatedier/frp/pkg/util/log"
	netpkg "github.com/fatedier/frp/pkg/util/net"
	"github.com/fatedier/frp/pkg/util/xlog"
)

type mixServerConfig struct {
	protocols map[string]v1.MixProtocolConfig
	udpDemux  *tmix.UDPDemux
	sshConfig *ssh.ServerConfig
}

func (svr *Service) initMixTransport() error {
	if !svr.cfg.IsMixEnabled() {
		return nil
	}
	entries, err := v1.ParseMixToken(svr.cfg.MixToken)
	if err != nil {
		return err
	}
	mcfg := &mixServerConfig{
		protocols: make(map[string]v1.MixProtocolConfig, len(entries)),
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		mcfg.protocols[entry.Protocol] = entry
		names = append(names, entry.Protocol)
	}
	log.Infof("mix init, bind port [%d], protocols %v", svr.cfg.MixBindPort, names)

	if proto, ok := mcfg.protocols[v1.MixProtocolSSH]; ok {
		keyBytes, err := transport.NewRandomPrivateKey()
		if err != nil {
			return err
		}
		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return err
		}
		mcfg.sshConfig = tmix.NewSSHServerConfig(proto.Username, proto.Password, signer)
	}

	address := net.JoinHostPort(svr.cfg.BindAddr, strconv.Itoa(svr.cfg.MixBindPort))
	svr.mixListener, err = net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on mix tcp address %s error: %v", address, err)
	}
	log.Infof("frps mix tcp listen on %s", address)

	svr.mixUDPConn, err = net.ListenPacket("udp", address)
	if err != nil {
		return fmt.Errorf("listen on mix udp address %s error: %v", address, err)
	}
	log.Infof("frps mix udp listen on %s", address)

	mcfg.udpDemux = tmix.NewUDPDemux(
		svr.mixUDPConn,
		tmix.RouteQUICThenKCP,
		svr.cfg.Transport.MaxUDPPendingPeers,
		svr.cfg.Transport.MaxUDPPeerRoutes,
		"quic", "kcp",
	)
	if _, ok := mcfg.protocols[v1.MixProtocolKCP]; ok {
		ln, err := kcp.ServeConn(nil, 10, 3, tmix.MustPacketConn(mcfg.udpDemux, "kcp"))
		if err != nil {
			return err
		}
		svr.mixKCPListener = ln
	}
	if _, ok := mcfg.protocols[v1.MixProtocolQUIC]; ok {
		quicTLSCfg := svr.tlsConfig.Clone()
		quicTLSCfg.NextProtos = []string{"frp"}
		transport := &quic.Transport{Conn: tmix.MustPacketConn(mcfg.udpDemux, "quic")}
		ln, err := transport.Listen(quicTLSCfg, &quic.Config{
			MaxIdleTimeout:     time.Duration(svr.cfg.Transport.QUIC.MaxIdleTimeout) * time.Second,
			MaxIncomingStreams: int64(svr.cfg.Transport.QUIC.MaxIncomingStreams),
			KeepAlivePeriod:    time.Duration(svr.cfg.Transport.QUIC.KeepalivePeriod) * time.Second,
		})
		if err != nil {
			return err
		}
		svr.mixQUICListener = ln
	}
	svr.mixConfig = mcfg
	return nil
}

func (svr *Service) runMixTransport() {
	if svr.mixConfig == nil {
		return
	}
	if svr.mixListener != nil {
		go svr.handleMixTCPListener(svr.mixListener)
	}
	if svr.mixConfig.udpDemux != nil {
		go func() {
			if err := svr.mixConfig.udpDemux.Serve(); err != nil {
				log.Warnf("mix udp demux stopped: %v", err)
			}
		}()
	}
	if svr.mixKCPListener != nil {
		go svr.handleMixKCPListener(svr.mixKCPListener, svr.mixConfig.protocols[v1.MixProtocolKCP])
	}
	if svr.mixQUICListener != nil {
		go svr.handleMixQUICListener(svr.mixQUICListener, svr.mixConfig.protocols[v1.MixProtocolQUIC])
	}
}

func (svr *Service) closeMixTransport() {
	if svr.mixListener != nil {
		_ = svr.mixListener.Close()
	}
	if svr.mixKCPListener != nil {
		_ = svr.mixKCPListener.Close()
	}
	if svr.mixQUICListener != nil {
		_ = svr.mixQUICListener.Close()
	}
	if svr.mixConfig != nil && svr.mixConfig.udpDemux != nil {
		_ = svr.mixConfig.udpDemux.Close()
	}
}

func (svr *Service) handleMixTCPListener(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Warnf("mix tcp listener closed")
			return
		}
		xl := xlog.New()
		ctx := xlog.NewContext(context.Background(), xl)
		conn = netpkg.NewContextConn(ctx, conn)
		go svr.handleMixTCPConn(ctx, conn)
	}
}

func (svr *Service) handleMixTCPConn(ctx context.Context, conn net.Conn) {
	sc, rd := gnet.NewSharedConnSize(conn, 512)
	peek := make([]byte, max(4, tmix.TokenMagicLen()))
	n, err := io.ReadAtLeast(rd, peek, 1)
	if err != nil {
		_ = conn.Close()
		return
	}
	peek = peek[:n]

	useMux := lo.FromPtr(svr.cfg.Transport.TCPMux)
	xl := xlog.FromContextSafe(ctx)

	if len(peek) >= 4 && string(peek[:4]) == "SSH-" && svr.mixConfig.sshConfig != nil {
		xl.Infof("mix selected protocol [ssh] for %s", conn.RemoteAddr())
		_ = tmix.ServeSSH(ctx, sc, svr.mixConfig.sshConfig, func(chCtx context.Context, c net.Conn) {
			go svr.serveAcceptedConn(chCtx, c, false, useMux)
		})
		return
	}

	if peek[0] == 0x16 || peek[0] == byte(netpkg.FRPTLSHeadByte) {
		if proto, ok := svr.mixConfig.protocols[v1.MixProtocolWSS]; ok {
			tlsConn, _, _, err := netpkg.CheckAndEnableTLSServerConnWithTimeout(sc, svr.tlsConfig, true, connReadTimeout)
			if err != nil {
				xl.Warnf("mix wss tls detect failed, continue probing: %v", err)
			} else {
				wsConn, err := tmix.AcceptWebsocketConn(tlsConn)
				if err != nil {
					xl.Warnf("mix wss accept failed, continue probing: %v", err)
				} else {
					if err := tmix.ReadAndVerifyToken(wsConn, proto.Protocol, proto.Password); err != nil {
						xl.Warnf("mix wss token verify failed: %v", err)
						_ = wsConn.Close()
						return
					}
					if err := tmix.WriteTokenAck(wsConn); err != nil {
						_ = wsConn.Close()
						return
					}
					xl.Infof("mix selected protocol [wss] for %s", conn.RemoteAddr())
					svr.serveAcceptedConn(ctx, wsConn, false, useMux)
					return
				}
			}
		}
	}

	if tmix.HasTokenMagic(peek) {
		proto, ok := svr.mixConfig.protocols[v1.MixProtocolTCP]
		if !ok {
			_ = conn.Close()
			return
		}
		if err := tmix.ReadAndVerifyToken(sc, proto.Protocol, proto.Password); err != nil {
			_ = sc.Close()
			return
		}
		if err := tmix.WriteTokenAck(sc); err != nil {
			_ = sc.Close()
			return
		}
		xl.Infof("mix selected protocol [tcp] for %s", conn.RemoteAddr())
		svr.serveAcceptedConn(ctx, sc, false, useMux)
		return
	}

	var ssErr error
	if proto, ok := svr.mixConfig.protocols[v1.MixProtocolSS]; ok {
		_ = conn.SetReadDeadline(time.Now().Add(mixDetectTimeout))
		ssConn, err := tmix.WrapShadowsocksServer(sc, proto.Method, proto.Password)
		_ = conn.SetReadDeadline(time.Time{})
		if err == nil {
			xl.Infof("mix selected protocol [ss] for %s", conn.RemoteAddr())
			svr.serveAcceptedConn(ctx, ssConn, false, useMux)
			return
		}
		ssErr = err
	}

	if ssErr != nil {
		xl.Warnf("mix shadowsocks detection failed: %v", ssErr)
	}

	_ = sc.Close()
}

func (svr *Service) handleMixKCPListener(l net.Listener, proto v1.MixProtocolConfig) {
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Warnf("mix kcp listener closed")
			return
		}
		kcpConn := conn.(*kcp.UDPSession)
		kcpConn.SetStreamMode(true)
		kcpConn.SetWriteDelay(true)
		kcpConn.SetNoDelay(1, 20, 2, 1)
		kcpConn.SetMtu(1350)
		kcpConn.SetWindowSize(1024, 1024)
		kcpConn.SetACKNoDelay(false)

		ctx := xlog.NewContext(context.Background(), xlog.New())
		go func() {
			routeID, ok := svr.mixConfig.udpDemux.RouteID(kcpConn.RemoteAddr(), proto.Protocol)
			if !ok {
				_ = kcpConn.Close()
				return
			}
			if err := tmix.ReadAndVerifyToken(kcpConn, proto.Protocol, proto.Password); err != nil {
				svr.mixConfig.udpDemux.Reject(kcpConn.RemoteAddr(), proto.Protocol, routeID)
				_ = kcpConn.Close()
				return
			}
			if !svr.mixConfig.udpDemux.Promote(kcpConn.RemoteAddr(), proto.Protocol, routeID) {
				_ = kcpConn.Close()
				return
			}
			if err := tmix.WriteTokenAck(kcpConn); err != nil {
				// Authentication already established this route. Keep it until its
				// normal TTL expires so an ACK failure cannot remove a reused route.
				_ = kcpConn.Close()
				return
			}
			xlog.FromContextSafe(ctx).Infof("mix selected protocol [kcp] for %s", kcpConn.RemoteAddr())
			svr.serveAcceptedConn(ctx, kcpConn, false, lo.FromPtr(svr.cfg.Transport.TCPMux))
		}()
	}
}

func (svr *Service) handleMixQUICListener(l *quic.Listener, proto v1.MixProtocolConfig) {
	for {
		c, err := l.Accept(context.Background())
		if err != nil {
			log.Warnf("mix quic listener closed")
			return
		}
		ctx := xlog.NewContext(context.Background(), xlog.New())
		go func(ctx context.Context, frpConn *quic.Conn) {
			routeID, ok := svr.mixConfig.udpDemux.RouteID(frpConn.RemoteAddr(), proto.Protocol)
			if !ok {
				_ = frpConn.CloseWithError(0, "")
				return
			}
			for {
				stream, err := frpConn.AcceptStream(context.Background())
				if err != nil {
					_ = frpConn.CloseWithError(0, "")
					return
				}
				go func() {
					conn := netpkg.QuicStreamToNetConn(stream, frpConn)
					if err := tmix.ReadAndVerifyToken(conn, proto.Protocol, proto.Password); err != nil {
						_ = frpConn.CloseWithError(0, "")
						svr.mixConfig.udpDemux.Reject(frpConn.RemoteAddr(), proto.Protocol, routeID)
						return
					}
					if !svr.mixConfig.udpDemux.Promote(frpConn.RemoteAddr(), proto.Protocol, routeID) {
						_ = frpConn.CloseWithError(0, "")
						return
					}
					if err := tmix.WriteTokenAck(conn); err != nil {
						// Promote is idempotent across streams. Leave the established
						// route to TTL cleanup instead of racing another valid stream.
						_ = frpConn.CloseWithError(0, "")
						return
					}
					xlog.FromContextSafe(ctx).Infof("mix selected protocol [quic] for %s", conn.RemoteAddr())
					svr.handleConnection(ctx, conn, false)
				}()
			}
		}(ctx, c)
	}
}
