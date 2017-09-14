package common

import (
	"net"
)

type Network struct {
}

func (s *Network) ReceiveInt(conn net.Conn) (int, error) {
	b := make([]byte, 4)
	n := 0
	for n < 4 {
		tmp, err := conn.Read(b[n:])
		if err != nil {
			plog.Error(err)
			return 0, err
		}
		n += tmp
	}
	return BytesToInt(b), nil
}

func (s *Network) ReceiveBytes(conn net.Conn, size int) ([]byte, error) {
	b := make([]byte, size)
	n := 0
	for n < size {
		tmp, err := conn.Read(b[n:])
		if err != nil {
			plog.Error(err)
			return b, err
		}
		n += tmp
	}
	return b, nil
}

func (s *Network) SendBytes(conn net.Conn, b []byte) error {
	n := 0
	for n < len(b) {
		tmp, err := conn.Write(b[n:])
		if err != nil {
			plog.Error(err)
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
