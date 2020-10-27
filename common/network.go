package common

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"golang.org/x/net/websocket"
	"io"
	"io/ioutil"
	"net"
	"time"
)

var (
	CIPHER_SUITES = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	}
	Network = new(network)
)

type network struct {
}

func (self *network) ReceiveInt(conn net.Conn) (int, error) {
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

func (self *network) ReceiveIntTimeout(conn net.Conn, timeout time.Duration) (int, error) {
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
	if err := self.ResetReadTimeout(conn); err != nil {
		return BytesToInt(b), err
	}
	return BytesToInt(b), nil
}

func (self *network) ReceiveBytes(conn net.Conn, size int) ([]byte, error) {
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

func (self *network) ReceiveBytesTimeout(conn net.Conn, size int, timeout time.Duration) ([]byte, error) {
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
	if err := self.ResetReadTimeout(conn); err != nil {
		return b, err
	}
	return b, nil
}

func (self *network) ResetReadTimeout(conn net.Conn) error {
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return err
	}
	return nil
}

func (self *network) ResetWriteTimeout(conn net.Conn) error {
	if err := conn.SetWriteDeadline(time.Time{}); err != nil {
		return err
	}
	return nil
}

func (self *network) SendBytes(conn net.Conn, b []byte) error {
	n := 0
	// TODO(chenglch): A workaround to solve 1006 error from websocket at
	// frontend side due to UTF8 encoding problem.
	if _, ok := conn.(*websocket.Conn); ok {
		s := base64.StdEncoding.EncodeToString(b)
		b = []byte(s)
	}
	for n < len(b) {
		tmp, err := conn.Write(b[n:])
		if err != nil {
			return err
		}
		n += tmp
	}
	return nil
}

func (self *network) SendInt(conn net.Conn, num int) error {
	b := IntToBytes(num)
	return self.SendBytes(conn, b)
}

func (self *network) SendIntWithTimeout(conn net.Conn, num int, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout * time.Second)); err != nil {
		return err
	}
	b := IntToBytes(num)
	if err := self.SendBytes(conn, b); err != nil {
		return err
	}
	return self.ResetWriteTimeout(conn)
}

func (self *network) SendBytesWithTimeout(conn net.Conn, b []byte, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout * time.Second)); err != nil {
		return err
	}
	if err := self.SendBytes(conn, b); err != nil {
		return err
	}
	return self.ResetWriteTimeout(conn)
}

func (self *network) SendByteWithLength(conn net.Conn, b []byte) error {
	lenBytes := IntToBytes(len(b))
	b = append(lenBytes, b...)
	return self.SendBytes(conn, b)
}

func (self *network) SendByteWithLengthTimeout(conn net.Conn, b []byte, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout * time.Second)); err != nil {
		return err
	}
	if err := self.SendByteWithLength(conn, b); err != nil {
		return err
	}
	return self.ResetWriteTimeout(conn)
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
