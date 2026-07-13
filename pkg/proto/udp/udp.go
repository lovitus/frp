// Copyright 2017 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package udp

import (
	"net"
	"sync"
	"time"

	"github.com/fatedier/golib/errors"
	"github.com/fatedier/golib/pool"

	"github.com/fatedier/frp/pkg/msg"
	netpkg "github.com/fatedier/frp/pkg/util/net"
)

func NewUDPPacket(buf []byte, laddr, raddr *net.UDPAddr) *msg.UDPPacket {
	content := make([]byte, len(buf))
	copy(content, buf)
	return &msg.UDPPacket{
		Content:    content,
		LocalAddr:  laddr,
		RemoteAddr: raddr,
	}
}

func GetContent(m *msg.UDPPacket) (buf []byte, err error) {
	return m.Content, nil
}

func ForwardUserConn(udpConn *net.UDPConn, readCh <-chan *msg.UDPPacket, sendCh chan<- *msg.UDPPacket, bufSize int) {
	// read
	go func() {
		for udpMsg := range readCh {
			buf, err := GetContent(udpMsg)
			if err != nil {
				continue
			}
			_, _ = udpConn.WriteToUDP(buf, udpMsg.RemoteAddr)
		}
	}()

	// write
	buf := pool.GetBuf(bufSize)
	defer pool.PutBuf(buf)
	for {
		n, remoteAddr, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		// NewUDPPacket copies buf[:n], so the read buffer can be reused
		udpMsg := NewUDPPacket(buf[:n], nil, remoteAddr)

		select {
		case sendCh <- udpMsg:
		default:
		}
	}
}

func Forwarder(dstAddr *net.UDPAddr, readCh <-chan *msg.UDPPacket, sendCh chan<- msg.Message, bufSize int, proxyProtocolVersion string, maxSessions int) {
	ForwarderWithLimiter(dstAddr, readCh, sendCh, bufSize, proxyProtocolVersion, NewSessionLimiter(maxSessions))
}

func ForwarderWithLimiter(dstAddr *net.UDPAddr, readCh <-chan *msg.UDPPacket, sendCh chan<- msg.Message, bufSize int, proxyProtocolVersion string, limiter *SessionLimiter) {
	const maxPendingPackets = 8
	var mu sync.RWMutex
	type session struct {
		mu      sync.Mutex
		conn    *net.UDPConn
		pending [][]byte
	}
	udpConnMap := make(map[string]*session)

	// read from dstAddr and write to sendCh
	writerFn := func(raddr *net.UDPAddr, s *session) {
		addr := raddr.String()
		defer func() {
			mu.Lock()
			if udpConnMap[addr] == s {
				delete(udpConnMap, addr)
			}
			mu.Unlock()
			s.conn.Close()
			limiter.Release()
		}()

		buf := pool.GetBuf(bufSize)
		defer pool.PutBuf(buf)
		for {
			_ = s.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, _, err := s.conn.ReadFromUDP(buf)
			if err != nil {
				return
			}

			udpMsg := NewUDPPacket(buf[:n], nil, raddr)
			if err = errors.PanicToError(func() {
				select {
				case sendCh <- udpMsg:
				default:
				}
			}); err != nil {
				return
			}
		}
	}

	// read from readCh
	go func() {
		defer func() {
			mu.Lock()
			sessions := make([]*session, 0, len(udpConnMap))
			for key, s := range udpConnMap {
				delete(udpConnMap, key)
				sessions = append(sessions, s)
			}
			mu.Unlock()

			for _, s := range sessions {
				s.mu.Lock()
				conn := s.conn
				s.mu.Unlock()
				if conn != nil {
					_ = conn.Close()
				}
			}
		}()

		for udpMsg := range readCh {
			if udpMsg.RemoteAddr == nil {
				continue
			}
			buf, err := GetContent(udpMsg)
			if err != nil {
				continue
			}

			key := udpMsg.RemoteAddr.String()
			mu.RLock()
			s := udpConnMap[key]
			mu.RUnlock()
			if s != nil {
				s.mu.Lock()
				if s.conn != nil {
					_, _ = s.conn.Write(buf)
				} else if len(s.pending) < maxPendingPackets {
					s.pending = append(s.pending, append([]byte(nil), buf...))
				}
				s.mu.Unlock()
				continue
			}

			mu.Lock()
			if udpConnMap[key] != nil || !limiter.TryAcquire() {
				mu.Unlock()
				continue
			}
			s = &session{pending: [][]byte{append([]byte(nil), buf...)}}
			udpConnMap[key] = s
			mu.Unlock()

			raddr := udpMsg.RemoteAddr
			go func() {
				udpConn, dialErr := net.DialUDP("udp", nil, dstAddr)
				if dialErr != nil {
					mu.Lock()
					if udpConnMap[key] == s {
						delete(udpConnMap, key)
					}
					mu.Unlock()
					limiter.Release()
					return
				}
				mu.Lock()
				if udpConnMap[key] != s {
					mu.Unlock()
					udpConn.Close()
					limiter.Release()
					return
				}
				s.mu.Lock()
				mu.Unlock()
				for i, payload := range s.pending {
					if i == 0 && proxyProtocolVersion != "" {
						ppBuf, err := netpkg.BuildProxyProtocolHeader(raddr, dstAddr, proxyProtocolVersion)
						if err == nil {
							payload = append(ppBuf, payload...)
						}
					}
					if _, err := udpConn.Write(payload); err != nil {
						break
					}
				}
				s.pending = nil
				s.conn = udpConn
				s.mu.Unlock()
				writerFn(raddr, s)
			}()
		}
	}()
}
