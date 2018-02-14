package logger

import (
	"fmt"
	"github.com/xcat2/goconserver/common"
	"net"
	"time"
)

const (
	UDP_PUBLISHER = "udp"
)

func init() {
	PUBLISHER_INIT_MAP[UDP_PUBLISHER] = NewUDPPublisher
}

func NewUDPPublisher(params interface{}) (Publisher, error) {
	var udpCfg *common.UDPCfg
	var err error
	var ok bool
	if udpCfg, ok = params.(*common.UDPCfg); !ok {
		return nil, common.ErrInvalidParameter
	}
	publisher := new(UDPPublisher)
	publisher.name = udpCfg.Name
	if publisher.name == "" {
		publisher.name = UNKNOWN
	}
	_, err = net.ResolveUDPAddr("udp", udpCfg.Host+":"+udpCfg.Port)
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	publisher.publisherChan = make(chan []byte, serverConfig.Global.Worker*2)
	publisher.udpCfg = udpCfg
	// not sure if one worker is enough for the big cluster
	go publisher.run()
	return publisher, nil
}

type UDPPublisher struct {
	NetworkPublisher
	udpCfg *common.UDPCfg
}

func (self *UDPPublisher) connect(connPtr *net.Conn) error {
	var err error
	*connPtr, err = net.DialTimeout(
		"udp",
		self.udpCfg.Host+":"+self.udpCfg.Port,
		time.Duration(self.udpCfg.Timeout)*time.Second)
	if err != nil {
		*connPtr = nil
		return err
	}
	return nil
}

func (self *UDPPublisher) emit(connPtr *net.Conn, b []byte) error {
	var err error
	timeout := time.Duration(self.udpCfg.Timeout)
	if *connPtr == nil {
		err = self.connect(connPtr)
		if err != nil {
			*connPtr = nil
			return err
		}
	}
	err = common.Network.SendBytesWithTimeout(*connPtr, b, timeout)
	if err != nil {
		plog.Debug(fmt.Sprintf("UDP publisher %s: connection disconnected, reconnecting...", self.name))
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

func (self *UDPPublisher) run() {
	var err error
	connPtr := new(net.Conn)
	plog.Info(fmt.Sprintf("Starting UDP publisher: %s", self.name))
	for {
		b := <-self.publisherChan
		err = self.emit(connPtr, b)
		if err != nil {
			plog.Error(fmt.Sprintf("UDP publisher %s: %s", self.name, err))
		}
	}
}
