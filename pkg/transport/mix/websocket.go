package mix

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	netpkg "github.com/fatedier/frp/pkg/util/net"
	"golang.org/x/net/websocket"
)

type singleConnListener struct {
	conn      net.Conn
	addr      net.Addr
	once      sync.Once
	closeOnce sync.Once
	closed    chan struct{}
}

func newSingleConnListener(conn net.Conn) net.Listener {
	return &singleConnListener{
		conn:   conn,
		addr:   conn.LocalAddr(),
		closed: make(chan struct{}),
	}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	var c net.Conn
	var ok bool
	l.once.Do(func() {
		c = l.conn
		ok = true
	})
	if ok {
		return c, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *singleConnListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
	})
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.addr
}

func AcceptWebsocketConn(conn net.Conn) (net.Conn, error) {
	ln := newSingleConnListener(conn)
	defer ln.Close()

	acceptedCh := make(chan net.Conn, 1)
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			websocket.Handler(func(ws *websocket.Conn) {
				notifyCh := make(chan struct{})
				wrapped := netpkg.WrapCloseNotifyConn(ws, func(_ error) {
					close(notifyCh)
				})
				acceptedCh <- wrapped
				<-notifyCh
			}).ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ln)
	}()

	select {
	case ws := <-acceptedCh:
		return ws, nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil, net.ErrClosed
		}
		return nil, err
	case <-time.After(10 * time.Second):
		_ = server.Close()
		return nil, net.ErrClosed
	}
}

func DialWebsocketConn(address, host string, tlsConfig *tls.Config) (net.Conn, error) {
	scheme := "ws"
	if tlsConfig != nil {
		scheme = "wss"
	}
	if host == "" {
		host = address
	}
	target := scheme + "://" + host + "/~!frp"
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	origin := "http://" + u.Host
	cfg, err := websocket.NewConfig(target, origin)
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		cfg.TlsConfig = tlsConfig
	}
	return websocket.DialConfig(cfg)
}
