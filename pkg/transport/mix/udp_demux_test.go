package mix

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDemuxPacketConnEnqueueAfterCloseIsDropped(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()

	demux := NewUDPDemux(pc, func([]byte) string { return "kcp" }, "kcp")
	conn := demux.Conn("kcp").(*demuxPacketConn)

	require.NoError(t, conn.Close())
	require.False(t, conn.enqueue(packet{
		data: []byte("payload"),
		addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}))
}
