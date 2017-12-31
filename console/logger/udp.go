package logger

import (
	"bytes"
	"github.com/chenglch/goconserver/common"
	"net"
	"time"
)

const (
	UDP_LOGGER = "udp_logger"
)

func init() {
	LOGGER_INIT_MAP[UDP_LOGGER] = NewUDPLogger
}

func NewUDPLogger() (Logger, error) {
	logger := new(UDPLogger)
	logger.bufChans = make(chan []byte, serverConfig.Global.Worker)
	// not sure if one worker is enough for the big cluster
	go logger.run()
	return logger, nil
}

type UDPLogger struct {
	LineLogger
}

func (self *UDPLogger) connect(connPtr *net.Conn) error {
	var err error
	*connPtr, err = net.DialTimeout(
		"udp",
		serverConfig.Console.UDPLogger.Host+":"+serverConfig.Console.UDPLogger.Port,
		time.Duration(serverConfig.Console.UDPLogger.Timeout)*time.Second)
	if err != nil {
		*connPtr = nil
		return err
	}
	return nil
}

func (self *UDPLogger) publish(connPtr *net.Conn, b []byte) error {
	var err error
	timeout := time.Duration(serverConfig.Console.UDPLogger.Timeout)
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

func (self *UDPLogger) run() {
	var buf bytes.Buffer
	var err error
	plog.Debug("Starting logstash publisher goroutine")
	ticker := time.NewTicker(sendInterval)
	sendChan := make(chan []byte, serverConfig.Console.UDPLogger.Timeout)
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
