package common

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"io/ioutil"
	"net"
	"time"
)

var (
	CIPHER_SUITES = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_RC4_128_SHA,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	}
)

type Network struct {
}

func (s *Network) ReceiveInt(conn net.Conn) (int, error) {
	b := make([]byte, 4)
	n := 0
	for n < 4 {
		tmp, err := conn.Read(b[n:])
		if err != nil {
			if err == io.EOF && tmp+n == 4 {
				return BytesToInt(b), err
			}
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
			if err == io.EOF && tmp+n == 4 {
				return BytesToInt(b), err
			}
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

func (s *Network) SendIntWithTimeout(conn net.Conn, num int, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout * time.Second)); err != nil {
		return err
	}
	b := IntToBytes(num)
	if err := s.SendBytes(conn, b); err != nil {
		return err
	}
	return s.ResetWriteTimeout(conn)
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

func LoadClientTlsConfig(certPath string, keyPath string, caCertPath string, serverHost string, insecure bool) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	pool.AppendCertsFromPEM(caCert)
	tlsConfig := tls.Config{RootCAs: pool, Certificates: []tls.Certificate{cert}, ServerName: serverHost,
		CipherSuites:             CIPHER_SUITES,
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		InsecureSkipVerify:       insecure,
	}
	tlsConfig.BuildNameToCertificate()
	return &tlsConfig, nil
}

func LoadServerTlsConfig(certPath string, keyPath string, caCertPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	pool.AppendCertsFromPEM(caCert)
	tlsConfig := tls.Config{ClientCAs: pool, Certificates: []tls.Certificate{cert},
		ClientAuth:               tls.RequireAnyClientCert,
		CipherSuites:             CIPHER_SUITES,
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true}
	tlsConfig.Rand = rand.Reader
	return &tlsConfig, nil
}
