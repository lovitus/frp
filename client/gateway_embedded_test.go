package client

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sagernet/sing/common/buf"
)

func TestRelayGatewayTCPAndCloseUnblocksOnPeerClose(t *testing.T) {
	t.Parallel()

	left, right := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- relayGatewayTCPAndClose(left, right)
	}()

	require.NoError(t, left.Close())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("relay did not exit after peer close")
	}
}

func TestGatewaySingSSResponseBufferHas2022Headroom(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		payloadLen := 512
		buffer := buf.NewSize(gatewaySingSSPacketHeadroom + payloadLen + gatewaySingSSPacketRearHeadroom)
		defer buffer.Release()
		buffer.Resize(gatewaySingSSPacketHeadroom, 0)
		_, err := buffer.Write(make([]byte, payloadLen))
		require.NoError(t, err)
		buffer.ExtendHeader(gatewaySingSSPacketHeadroom)
		buffer.Extend(gatewaySingSSPacketRearHeadroom)
	})
}
