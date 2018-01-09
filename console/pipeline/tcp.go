package logger

import (
	"crypto/tls"
	"fmt"
	"github.com/chenglch/goconserver/common"
	"net"
	"time"
)

const (
	TCP_PUBLISHER = "tcp"
)

func init() {
	PUBLISHER_INIT_MAP[TCP_PUBLISHER] = NewTCPPublisher
}

func NewTCPPublisher(params interface{}) (Publisher, error) {
	var err error
	var tcpCfg *common.TCPCfg
	var ok bool
	if tcpCfg, ok = params.(*common.TCPCfg); !ok {
		return nil, common.ErrInvalidParameter
	}
	_, err = net.ResolveTCPAddr("tcp", tcpCfg.Host+":"+tcpCfg.Port)
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	publisher := new(TCPPublisher)
	publisher.name = tcpCfg.Name
	if publisher.name == "" {
		publisher.name = UNKNOWN
	}
	publisher.publisherChan = make(chan []byte, serverConfig.Global.Worker*2)
	publisher.tcpCfg = tcpCfg

	// not sure if one worker is enough for the big cluster
	go publisher.run()
	return publisher, nil
}

type TCPPublisher struct {
	NetworkPublisher
	tcpCfg *common.TCPCfg
}

func (self *TCPPublisher) connect(connPtr *net.Conn) error {
	var err error
	*connPtr, err = net.DialTimeout(
		"tcp",
		self.tcpCfg.Host+":"+self.tcpCfg.Port,
		time.Duration(self.tcpCfg.Timeout)*time.Second)
	if err != nil {
		*connPtr = nil
		return err
	}

	if self.tcpCfg.SSLCertFile != "" &&
		self.tcpCfg.SSLKeyFile != "" &&
		self.tcpCfg.SSLCACertFile != "" {

		tlsConfig, err := common.LoadClientTlsConfig(
			self.tcpCfg.SSLCertFile,
			self.tcpCfg.SSLKeyFile,
			self.tcpCfg.SSLCACertFile,
			self.tcpCfg.Host,
			self.tcpCfg.SSLInsecure)
		if err != nil {
			plog.Debug(fmt.Sprintf("TCP publisher %s: %s", self.name, err))
			return err
		}
		// avoid of handshake timeout
		*connPtr = tls.Client(*connPtr, tlsConfig)
		if err := (*connPtr).SetDeadline(time.Now().Add(time.Duration(self.tcpCfg.Timeout) * time.Second)); err != nil {
			plog.Debug(fmt.Sprintf("TCP publisher %s: %s", self.name, err))
			return err
		}
		err = (*connPtr).(*tls.Conn).Handshake()
		if err != nil {
			plog.Debug(fmt.Sprintf("TCP publisher %s: %s", self.name, err))
			return err
		}
		if err = (*connPtr).SetDeadline(time.Time{}); err != nil {
			plog.Debug(fmt.Sprintf("TCP publisher %s: %s", self.name, err))
			return err
		}
	}
	return nil
}

func (self *TCPPublisher) emit(connPtr *net.Conn, b []byte) error {
	var err error
	timeout := time.Duration(self.tcpCfg.Timeout)
	if *connPtr == nil {
		err = self.connect(connPtr)
		if err != nil {
			*connPtr = nil
			return err
		}
	}
	err = common.Network.SendBytesWithTimeout(*connPtr, b, timeout)
	if err != nil {
		plog.Debug(fmt.Sprintf("TCP publisher %s: connection disconnected, reconnecting...", self.name))
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

func (self *TCPPublisher) run() {
	var err error
	connPtr := new(net.Conn)
	plog.Info(fmt.Sprintf("Starting TCP publisher: %s", self.name))
	for {
		b := <-self.publisherChan
		err = self.emit(connPtr, b)
		if err != nil {
			plog.Error(fmt.Sprintf("TCP publisher %s: %s", self.name, err))
		}
	}
}
