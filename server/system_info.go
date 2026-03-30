package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fatedier/frp/pkg/msg"
	"github.com/fatedier/frp/pkg/util/util"
)

const gatewaySystemInfoTimeout = 7 * time.Second

type gatewaySystemInfoWaiter struct {
	clientKey string
	ch        chan msg.GatewaySystemInfo
}

type SystemInfoManager struct {
	sendMessage func(string, msg.Message) error

	mu      sync.Mutex
	waiters map[string]*gatewaySystemInfoWaiter
}

func NewSystemInfoManager(sendMessage func(string, msg.Message) error) *SystemInfoManager {
	return &SystemInfoManager{
		sendMessage: sendMessage,
		waiters:     make(map[string]*gatewaySystemInfoWaiter),
	}
}

func (m *SystemInfoManager) HandleResponse(clientKey string, resp *msg.GatewaySystemInfoResponse) {
	if resp == nil || resp.RequestID == "" {
		return
	}

	m.mu.Lock()
	waiter, ok := m.waiters[resp.RequestID]
	m.mu.Unlock()
	if !ok || waiter.clientKey != clientKey {
		return
	}

	select {
	case waiter.ch <- resp.Info:
	default:
	}
}

func (m *SystemInfoManager) Request(ctx context.Context, clientKey string) (msg.GatewaySystemInfo, error) {
	if m.sendMessage == nil {
		return msg.GatewaySystemInfo{}, fmt.Errorf("system info request sender is unavailable")
	}
	requestID, err := util.RandID()
	if err != nil {
		return msg.GatewaySystemInfo{}, err
	}

	waiter := &gatewaySystemInfoWaiter{
		clientKey: clientKey,
		ch:        make(chan msg.GatewaySystemInfo, 1),
	}

	m.mu.Lock()
	m.waiters[requestID] = waiter
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.waiters, requestID)
		m.mu.Unlock()
	}()

	if err := m.sendMessage(clientKey, &msg.GatewaySystemInfoRequest{RequestID: requestID}); err != nil {
		return msg.GatewaySystemInfo{}, err
	}

	waitCtx, cancel := context.WithTimeout(ctx, gatewaySystemInfoTimeout)
	defer cancel()

	select {
	case info := <-waiter.ch:
		return info, nil
	case <-waitCtx.Done():
		return msg.GatewaySystemInfo{}, fmt.Errorf("gateway system info request timed out")
	}
}
