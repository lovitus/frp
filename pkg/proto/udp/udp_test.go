package udp

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/fatedier/frp/pkg/msg"
)

func TestUdpPacket(t *testing.T) {
	require := require.New(t)

	buf := []byte("hello world")
	udpMsg := NewUDPPacket(buf, nil, nil)

	newBuf, err := GetContent(udpMsg)
	require.NoError(err)
	require.EqualValues(buf, newBuf)
}

func TestForwarderReleasesSessionsWhenInputCloses(t *testing.T) {
	dst, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, dst.Close()) })

	readCh := make(chan *msg.UDPPacket)
	sendCh := make(chan msg.Message)
	limiter := NewSessionLimiter(1)
	ForwarderWithLimiter(dst.LocalAddr().(*net.UDPAddr), readCh, sendCh, 1500, "", limiter)

	readCh <- NewUDPPacket(
		[]byte("payload"),
		nil,
		&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345},
	)
	require.Eventually(t, func() bool { return limiter.Active() == 1 }, time.Second, 10*time.Millisecond)

	close(readCh)
	require.Eventually(t, func() bool { return limiter.Active() == 0 }, time.Second, 10*time.Millisecond)
}
