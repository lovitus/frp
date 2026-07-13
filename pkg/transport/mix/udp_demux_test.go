package mix

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDemuxPacketConnEnqueueAfterCloseIsDropped(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()

	demux := NewUDPDemux(pc, func([]byte) string { return "kcp" }, 10, 10, "kcp")
	conn := demux.Conn("kcp").(*demuxPacketConn)

	require.NoError(t, conn.Close())
	require.False(t, conn.enqueue(packet{
		data: []byte("payload"),
		addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}))
}

func TestUDPDemuxPruneStalePeers(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()

	demux := NewUDPDemux(pc, func([]byte) string { return "kcp" }, 10, 10, "kcp")
	conn := demux.Conn("kcp").(*demuxPacketConn)

	now := time.Now()
	demux.mu.Lock()
	demux.establishedPeers["stale"] = peerRoute{
		child:    conn,
		lastSeen: now.Add(-udpEstablishedPeerTTL - time.Second),
	}
	demux.establishedPeers["fresh"] = peerRoute{
		child:    conn,
		lastSeen: now,
	}
	demux.pruneEstablishedPeersLocked(now)
	_, staleExists := demux.establishedPeers["stale"]
	_, freshExists := demux.establishedPeers["fresh"]
	demux.mu.Unlock()

	require.False(t, staleExists)
	require.True(t, freshExists)
}

func TestUDPDemuxPromotionUsesRouteIdentityAndCapacity(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()

	demux := NewUDPDemux(pc, func([]byte) string { return "kcp" }, 2, 1, "kcp")
	child := demux.Conn("kcp").(*demuxPacketConn)
	addr1 := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001}
	addr2 := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10002}
	now := time.Now()

	demux.mu.Lock()
	demux.pendingPeers[addr1.String()] = peerRoute{id: 1, protocol: "kcp", child: child, createdAt: now}
	demux.pendingPeers[addr2.String()] = peerRoute{id: 2, protocol: "kcp", child: child, createdAt: now}
	demux.pendingCounts["kcp"] = 2
	demux.mu.Unlock()

	require.False(t, demux.Promote(addr1, "kcp", 99))
	require.True(t, demux.Promote(addr1, "kcp", 1))
	require.True(t, demux.Promote(addr1, "kcp", 1), "promotion is idempotent across streams")
	demux.Reject(addr1, "kcp", 1)
	retainedID, ok := demux.RouteID(addr1, "kcp")
	require.True(t, ok, "reject never removes an established route")
	require.Equal(t, uint64(1), retainedID)
	require.False(t, demux.Promote(addr2, "kcp", 2))

	demux.Reject(addr2, "kcp", 1)
	_, ok = demux.RouteID(addr2, "kcp")
	require.False(t, ok, "a failed promotion at capacity releases its pending route")
}

func TestUDPDemuxExpiredPendingCannotPromote(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()

	demux := NewUDPDemux(pc, func([]byte) string { return "quic" }, 1, 1, "quic")
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10003}
	demux.pendingPeers[addr.String()] = peerRoute{
		id: 1, protocol: "quic", child: demux.Conn("quic").(*demuxPacketConn),
		createdAt: time.Now().Add(-udpPendingPeerTTL - time.Second),
	}
	demux.pendingCounts["quic"] = 1

	require.False(t, demux.Promote(addr, "quic", 1))
}
