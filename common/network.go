package common

import (
	"net"
	"time"
)

type Network struct {
}

func (s *Network) ReceiveInt(conn net.Conn) (int, error) {
	b := make([]byte, 4)
	n := 0
	for n < 4 {
		tmp, err := conn.Read(b[n:])
		if err != nil {
			return 0, err
		}
		n += tmp
	}
	return BytesToInt(b), nil
}

func (s *Network) ReceiveIntTimeout(conn net.Conn, timeout time.Duration) (int, error) {
	b := make([]byte, 4)
	n := 0
	for n < 4 {
		if err := conn.SetReadDeadline(time.Now().Add(timeout * time.Second)); err != nil {
			return 0, err
		}
		tmp, err := conn.Read(b[n:])
		if err != nil {
			return 0, err
		}
		n += tmp
	}
	if err := s.ResetReadTimeout(conn); err != nil {
		return BytesToInt(b), err
	}
	return BytesToInt(b), nil
}

func (s *Network) ReceiveBytes(conn net.Conn, size int) ([]byte, error) {
	b := make([]byte, size)
	n := 0
	for n < size {
		tmp, err := conn.Read(b[n:])
		if err != nil {
			return b, err
		}
		n += tmp
	}
	return b, nil
}

func (s *Network) ReceiveBytesTimeout(conn net.Conn, size int, timeout time.Duration) ([]byte, error) {
	b := make([]byte, size)
	n := 0
	for n < size {
		if err := conn.SetReadDeadline(time.Now().Add(timeout * time.Second)); err != nil {
			return b, err
		}
		tmp, err := conn.Read(b[n:])
		if err != nil {
			return b, err
		}
		n += tmp
	}
	if err := s.ResetReadTimeout(conn); err != nil {
		return b, err
	}
	return b, nil
}

func (s *Network) ResetReadTimeout(conn net.Conn) error {
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return err
	}
	return nil
}

func (s *Network) ResetWriteTimeout(conn net.Conn) error {
	if err := conn.SetWriteDeadline(time.Time{}); err != nil {
		return err
	}
	return nil
}

func (s *Network) SendBytes(conn net.Conn, b []byte) error {
	n := 0
	for n < len(b) {
		tmp, err := conn.Write(b[n:])
		if err != nil {
			return err
		}
		n += tmp
	}
	return nil
}

func (s *Network) SendInt(conn net.Conn, num int) error {
	b := IntToBytes(num)
	return s.SendBytes(conn, b)
}

func (s *Network) SendByteWithLength(conn net.Conn, b []byte) error {
	lenBytes := IntToBytes(len(b))
	b = append(lenBytes, b...)
	return s.SendBytes(conn, b)
}

func (s *Network) SendByteWithLengthTimeout(conn net.Conn, b []byte, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout * time.Second)); err != nil {
		return err
	}
	if err := s.SendByteWithLength(conn, b); err != nil {
		return err
	}
	return s.ResetWriteTimeout(conn)
}
