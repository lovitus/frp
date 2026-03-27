package mix

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

const SSHChannelType = "frp"

type SSHChannelConn struct {
	ssh.Channel
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (c *SSHChannelConn) LocalAddr() net.Addr              { return c.localAddr }
func (c *SSHChannelConn) RemoteAddr() net.Addr             { return c.remoteAddr }
func (c *SSHChannelConn) SetDeadline(time.Time) error      { return nil }
func (c *SSHChannelConn) SetReadDeadline(time.Time) error  { return nil }
func (c *SSHChannelConn) SetWriteDeadline(time.Time) error { return nil }

func NewSSHServerConfig(username, password string, signer ssh.Signer) *ssh.ServerConfig {
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if username == "" {
				return nil, fmt.Errorf("missing ssh username")
			}
			if meta.User() != username {
				return nil, fmt.Errorf("invalid ssh username")
			}
			if string(pass) != password {
				return nil, fmt.Errorf("invalid ssh password")
			}
			return nil, nil
		},
	}
	cfg.AddHostKey(signer)
	return cfg
}

func DialSSH(ctx context.Context, address, username, password string, timeout time.Duration) (*ssh.Client, net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, nil, err
	}
	cfg := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{ssh.Password(password)},
		//nolint:gosec // mix uses an ephemeral server host key generated per frps process; transport auth is handled separately.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	conn, chans, reqs, err := ssh.NewClientConn(rawConn, address, cfg)
	if err != nil {
		rawConn.Close()
		return nil, nil, err
	}
	return ssh.NewClient(conn, chans, reqs), rawConn, nil
}

func OpenSSHChannel(client *ssh.Client) (net.Conn, error) {
	ch, reqs, err := client.OpenChannel(SSHChannelType, nil)
	if err != nil {
		return nil, err
	}
	go ssh.DiscardRequests(reqs)
	return &SSHChannelConn{
		Channel:    ch,
		localAddr:  client.LocalAddr(),
		remoteAddr: client.RemoteAddr(),
	}, nil
}

func ServeSSH(ctx context.Context, rawConn net.Conn, cfg *ssh.ServerConfig, handle func(context.Context, net.Conn)) error {
	sshConn, chans, reqs, err := ssh.NewServerConn(rawConn, cfg)
	if err != nil {
		return err
	}
	go ssh.DiscardRequests(reqs)

	go func() {
		<-ctx.Done()
		_ = sshConn.Close()
	}()

	for newChannel := range chans {
		if newChannel.ChannelType() != SSHChannelType {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel")
			continue
		}
		ch, reqs, err := newChannel.Accept()
		if err != nil {
			return err
		}
		go ssh.DiscardRequests(reqs)
		handle(ctx, &SSHChannelConn{
			Channel:    ch,
			localAddr:  sshConn.LocalAddr(),
			remoteAddr: sshConn.RemoteAddr(),
		})
	}
	return nil
}
