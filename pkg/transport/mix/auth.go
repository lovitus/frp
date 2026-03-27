package mix

import (
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

var authMagic = []byte("FRPMIX1")

const (
	authReadTimeout = 2 * time.Second
	authAckByte     = 0x01
)

func WriteToken(conn net.Conn, protocol, password string) error {
	payloadLen := len(protocol) + len(password) + 3
	buf := make([]byte, 0, len(authMagic)+payloadLen)
	buf = append(buf, authMagic...)
	buf = append(buf, byte(len(protocol)))
	tmp := make([]byte, 2)
	binary.BigEndian.PutUint16(tmp, uint16(len(password)))
	buf = append(buf, tmp...)
	buf = append(buf, protocol...)
	buf = append(buf, password...)
	_, err := conn.Write(buf)
	return err
}

func HasTokenMagic(data []byte) bool {
	if len(data) < len(authMagic) {
		return false
	}
	return subtle.ConstantTimeCompare(data[:len(authMagic)], authMagic) == 1
}

func TokenMagicLen() int {
	return len(authMagic)
}

func ReadAndVerifyToken(conn net.Conn, expectedProtocol, expectedPassword string) error {
	_ = conn.SetReadDeadline(time.Now().Add(authReadTimeout))
	defer func() {
		_ = conn.SetReadDeadline(time.Time{})
	}()

	magic := make([]byte, len(authMagic))
	if _, err := io.ReadFull(conn, magic); err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(magic, authMagic) != 1 {
		return fmt.Errorf("invalid mix auth magic")
	}

	var protoLen [1]byte
	if _, err := io.ReadFull(conn, protoLen[:]); err != nil {
		return err
	}
	var passwordLenBuf [2]byte
	if _, err := io.ReadFull(conn, passwordLenBuf[:]); err != nil {
		return err
	}
	passwordLen := int(binary.BigEndian.Uint16(passwordLenBuf[:]))

	protocol := make([]byte, int(protoLen[0]))
	if _, err := io.ReadFull(conn, protocol); err != nil {
		return err
	}
	password := make([]byte, passwordLen)
	if _, err := io.ReadFull(conn, password); err != nil {
		return err
	}

	if subtle.ConstantTimeCompare(protocol, []byte(expectedProtocol)) != 1 {
		return fmt.Errorf("unexpected mix protocol %q", string(protocol))
	}
	if subtle.ConstantTimeCompare(password, []byte(expectedPassword)) != 1 {
		return fmt.Errorf("invalid mix credential")
	}
	return nil
}

func WriteTokenAck(conn net.Conn) error {
	_ = conn.SetWriteDeadline(time.Now().Add(authReadTimeout))
	defer func() {
		_ = conn.SetWriteDeadline(time.Time{})
	}()
	_, err := conn.Write([]byte{authAckByte})
	return err
}

func ReadTokenAck(conn net.Conn) error {
	_ = conn.SetReadDeadline(time.Now().Add(authReadTimeout))
	defer func() {
		_ = conn.SetReadDeadline(time.Time{})
	}()

	var ack [1]byte
	if _, err := io.ReadFull(conn, ack[:]); err != nil {
		return err
	}
	if ack[0] != authAckByte {
		return fmt.Errorf("invalid mix auth ack")
	}
	return nil
}
