package logger

import (
	"bytes"
	"crypto/tls"
	"github.com/chenglch/goconserver/common"
	"net"
	"time"
)

const (
	TCP_LOGGER = "tcp_logger"
)

func init() {
	LOGGER_INIT_MAP[TCP_LOGGER] = NewTCPLogger
}

func NewTCPLogger() (Logger, error) {
	var err error
	_, err = net.ResolveTCPAddr("tcp", serverConfig.Console.TCPLogger.Host+":"+serverConfig.Console.TCPLogger.Port)
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	logger := new(TCPLogger)
	logger.bufChans = make(chan []byte, serverConfig.Global.Worker)
	// not sure if one worker is enough for the big cluster
	go logger.run()
	return logger, nil
}

type TCPLogger struct {
	LineLogger
}

func (self *TCPLogger) connect(connPtr *net.Conn) error {
	var err error
	*connPtr, err = net.DialTimeout(
		"tcp",
		serverConfig.Console.TCPLogger.Host+":"+serverConfig.Console.TCPLogger.Port,
		time.Duration(serverConfig.Console.TCPLogger.Timeout)*time.Second)
	if err != nil {
		*connPtr = nil
		return err
	}

	if serverConfig.Console.TCPLogger.SSLCertFile != "" &&
		serverConfig.Console.TCPLogger.SSLKeyFile != "" &&
		serverConfig.Console.TCPLogger.SSLCACertFile != "" {

		tlsConfig, err := common.LoadClientTlsConfig(
			serverConfig.Console.TCPLogger.SSLCertFile,
			serverConfig.Console.TCPLogger.SSLKeyFile,
			serverConfig.Console.TCPLogger.SSLCACertFile,
			serverConfig.Console.TCPLogger.Host,
			serverConfig.Console.TCPLogger.SSLInsecure)
		if err != nil {
			plog.Error(err)
			return err
		}
		// avoid of handshake timeout
		*connPtr = tls.Client(*connPtr, tlsConfig)
		if err := (*connPtr).SetDeadline(time.Now().Add(time.Duration(serverConfig.Console.TCPLogger.Timeout) * time.Second)); err != nil {
			plog.Error(err)
			return err
		}
		err = (*connPtr).(*tls.Conn).Handshake()
		if err != nil {
			plog.Error(err)
			return err
		}
		if err = (*connPtr).SetDeadline(time.Time{}); err != nil {
			plog.Error(err)
			return err
		}
	}
	return nil
}

func (self *TCPLogger) publish(connPtr *net.Conn, b []byte) error {
	var err error
	timeout := time.Duration(serverConfig.Console.TCPLogger.Timeout)
	if *connPtr == nil {
		err = self.connect(connPtr)
		if err != nil {
			*connPtr = nil
			return err
		}
	}
	err = common.Network.SendBytesWithTimeout(*connPtr, b, timeout)
	if err != nil {
		plog.Debug("Logger connection disconnected, reconnecting...")
		err = self.connect(connPtr)
		if err != nil {
			*connPtr = nil
			return err
		}
		err = common.Network.SendBytesWithTimeout(*connPtr, b, timeout)
	}
	if err != nil {
		return err
	}
	return nil
}

func (self *TCPLogger) run() {
	var buf bytes.Buffer
	var err error
	plog.Debug("Starting logstash publisher goroutine")
	ticker := time.NewTicker(sendInterval)
	// timeout may block data
	sendChan := make(chan []byte, serverConfig.Console.TCPLogger.Timeout*2)
	go func(send <-chan []byte) {
		connPtr := new(net.Conn)
		for {
			b := <-send
			err = self.publish(connPtr, b)
			if err != nil {
				plog.Error(err)
			}
		}
	}(sendChan)
	for {
		select {
		case b := <-self.bufChans:
			buf.Write(b)
		case <-ticker.C:
			if buf.Len() > 0 {
				sendChan <- buf.Bytes()
				buf.Reset()
			}
		}
	}
}
