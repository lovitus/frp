package mix

import (
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	udpPendingPeerTTL           = 10 * time.Second
	udpPendingPeerPruneInterval = time.Second
	udpEstablishedPeerTTL       = 5 * time.Minute
	udpEstablishedPruneInterval = time.Minute
)

type packet struct {
	data []byte
	addr net.Addr
}

type demuxPacketConn struct {
	parent *UDPDemux
	name   string
	ch     chan packet

	mu           sync.RWMutex
	readDeadline time.Time
	closed       bool
}

func (c *demuxPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		c.mu.RLock()
		deadline := c.readDeadline
		closed := c.closed
		c.mu.RUnlock()
		if closed {
			return 0, nil, net.ErrClosed
		}

		var timer <-chan time.Time
		if !deadline.IsZero() {
			d := time.Until(deadline)
			if d <= 0 {
				return 0, nil, net.ErrClosed
			}
			timer = time.After(d)
		}

		select {
		case pkt, ok := <-c.ch:
			if !ok {
				return 0, nil, net.ErrClosed
			}
			n := copy(p, pkt.data)
			return n, pkt.addr, nil
		case <-timer:
			return 0, nil, net.ErrClosed
		}
	}
}

func (c *demuxPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	return c.parent.conn.WriteTo(p, addr)
}

func (c *demuxPacketConn) enqueue(pkt packet) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return false
	}
	select {
	case c.ch <- pkt:
		return true
	default:
		return false
	}
}

func (c *demuxPacketConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	close(c.ch)
	return nil
}

func (c *demuxPacketConn) LocalAddr() net.Addr {
	return c.parent.conn.LocalAddr()
}

func (c *demuxPacketConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readDeadline = t
	return nil
}

func (c *demuxPacketConn) SetReadDeadline(t time.Time) error {
	return c.SetDeadline(t)
}

func (c *demuxPacketConn) SetWriteDeadline(time.Time) error {
	return nil
}

type UDPDemux struct {
	conn net.PacketConn

	mu                   sync.RWMutex
	routeFn              func([]byte) string
	pendingPeers         map[string]peerRoute
	establishedPeers     map[string]peerRoute
	pendingCounts        map[string]int
	conns                map[string]*demuxPacketConn
	maxPendingPeers      int
	maxPeerRoutes        int
	nextRouteID          uint64
	lastPendingPrune     time.Time
	lastEstablishedPrune time.Time
	closed               bool
}

type peerRoute struct {
	id        uint64
	protocol  string
	child     *demuxPacketConn
	createdAt time.Time
	lastSeen  time.Time
}

func NewUDPDemux(conn net.PacketConn, routeFn func([]byte) string, maxPendingPeers, maxPeerRoutes int, names ...string) *UDPDemux {
	now := time.Now()
	d := &UDPDemux{
		conn:                 conn,
		routeFn:              routeFn,
		pendingPeers:         make(map[string]peerRoute),
		establishedPeers:     make(map[string]peerRoute),
		pendingCounts:        make(map[string]int),
		conns:                make(map[string]*demuxPacketConn, len(names)),
		maxPendingPeers:      maxPendingPeers,
		maxPeerRoutes:        maxPeerRoutes,
		lastPendingPrune:     now,
		lastEstablishedPrune: now,
	}
	for _, name := range names {
		d.conns[name] = &demuxPacketConn{
			parent: d,
			name:   name,
			ch:     make(chan packet, 256),
		}
	}
	return d
}

func (d *UDPDemux) Conn(name string) net.PacketConn {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.conns[name]
}

func (d *UDPDemux) Serve() error {
	buf := make([]byte, 64*1024)
	for {
		n, addr, err := d.conn.ReadFrom(buf)
		if err != nil {
			return err
		}
		key := addr.String()
		now := time.Now()

		d.mu.Lock()
		if now.Sub(d.lastPendingPrune) >= udpPendingPeerPruneInterval {
			d.prunePendingPeersLocked(now)
			d.lastPendingPrune = now
		}
		if now.Sub(d.lastEstablishedPrune) >= udpEstablishedPruneInterval {
			d.pruneEstablishedPeersLocked(now)
			d.lastEstablishedPrune = now
		}

		route, established := d.establishedPeers[key]
		if established {
			// Established routes follow UDP peer activity, not a stream or connection
			// callback. Keeping the route until its idle TTL also ensures late QUIC
			// short-header and close packets cannot be reclassified as KCP.
			route.lastSeen = now
			d.establishedPeers[key] = route
		} else {
			route = d.pendingPeers[key]
		}
		child := route.child
		if child == nil {
			name := d.routeFn(buf[:n])
			child = d.conns[name]
			if child == nil || d.pendingCounts[name] >= d.maxPendingPeers {
				d.mu.Unlock()
				continue
			}
			d.nextRouteID++
			route = peerRoute{id: d.nextRouteID, protocol: name, child: child, createdAt: now, lastSeen: now}
			d.pendingPeers[key] = route
			d.pendingCounts[name]++
		}
		d.mu.Unlock()

		pkt := packet{
			data: append([]byte(nil), buf[:n]...),
			addr: addr,
		}
		_ = child.enqueue(pkt)
	}
}

func (d *UDPDemux) RouteID(addr net.Addr, protocol string) (uint64, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	key := addr.String()
	if route, ok := d.pendingPeers[key]; ok && route.protocol == protocol {
		return route.id, true
	}
	if route, ok := d.establishedPeers[key]; ok && route.protocol == protocol {
		return route.id, true
	}
	return 0, false
}

func (d *UDPDemux) Promote(addr net.Addr, protocol string, routeID uint64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	key := addr.String()
	if route, ok := d.establishedPeers[key]; ok {
		return route.id == routeID && route.protocol == protocol
	}
	route, ok := d.pendingPeers[key]
	if !ok || route.id != routeID || route.protocol != protocol {
		return false
	}
	if time.Since(route.createdAt) > udpPendingPeerTTL {
		delete(d.pendingPeers, key)
		d.pendingCounts[protocol]--
		return false
	}
	delete(d.pendingPeers, key)
	d.pendingCounts[protocol]--
	if len(d.establishedPeers) >= d.maxPeerRoutes {
		return false
	}
	route.lastSeen = time.Now()
	d.establishedPeers[key] = route
	return true
}

func (d *UDPDemux) Reject(addr net.Addr, protocol string, routeID uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	key := addr.String()
	// Authentication rejection only owns the matching pending route. An
	// established route may already be shared by other QUIC streams and is
	// intentionally reclaimed by its bounded idle TTL.
	if route, ok := d.pendingPeers[key]; ok && route.id == routeID && route.protocol == protocol {
		delete(d.pendingPeers, key)
		d.pendingCounts[protocol]--
	}
}

func (d *UDPDemux) prunePendingPeersLocked(now time.Time) {
	for key, route := range d.pendingPeers {
		if now.Sub(route.createdAt) > udpPendingPeerTTL {
			delete(d.pendingPeers, key)
			d.pendingCounts[route.protocol]--
		}
	}
}

func (d *UDPDemux) pruneEstablishedPeersLocked(now time.Time) {
	stale := 0
	for _, route := range d.establishedPeers {
		if now.Sub(route.lastSeen) > udpEstablishedPeerTTL {
			stale++
		}
	}
	if stale == 0 {
		return
	}
	if stale*2 >= len(d.establishedPeers) {
		next := make(map[string]peerRoute, len(d.establishedPeers)-stale)
		for key, route := range d.establishedPeers {
			if now.Sub(route.lastSeen) <= udpEstablishedPeerTTL {
				next[key] = route
			}
		}
		d.establishedPeers = next
		return
	}
	for key, route := range d.establishedPeers {
		if now.Sub(route.lastSeen) > udpEstablishedPeerTTL {
			delete(d.establishedPeers, key)
		}
	}
}

func (d *UDPDemux) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	for _, child := range d.conns {
		_ = child.Close()
	}
	return d.conn.Close()
}

func RouteQUICThenKCP(packet []byte) string {
	if len(packet) == 0 {
		return ""
	}
	// QUIC Initial packets always carry both Header Form (0x80) and Fixed Bit (0x40).
	if packet[0]&0x80 != 0 && packet[0]&0x40 != 0 {
		return "quic"
	}
	return "kcp"
}

func MustPacketConn(d *UDPDemux, name string) net.PacketConn {
	conn := d.Conn(name)
	if conn == nil {
		panic(fmt.Sprintf("missing demux packet conn %q", name))
	}
	return conn
}
