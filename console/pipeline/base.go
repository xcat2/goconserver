package logger

import (
	"github.com/chenglch/goconserver/common"
	"time"
)

const (
	sendInterval = 300 * time.Millisecond
	LINE_LOGGER  = "line"
	BYTE_LOGGER  = "byte"
	UNKNOWN      = "unknown"
)

var (
	PUBLISHER_INIT_MAP = map[string]func(interface{}) (Publisher, error){}
	LOGGER_INIT_MAP    = map[string]func() Logger{}
	plog               = common.GetLogger("github.com/chenglch/goconserver/console/logger")
	serverConfig       = common.GetServerConfig()
)

type Logger interface {
	MakeRecord(node string, b []byte, last *[]byte) error // create log message
	Prompt(node string, message string) error             // prompt message about console event
	Fetch(node string, count int) (string, error)         // to support console replay
	Register(publisher Publisher)
}

type Publisher interface {
	Publish(node string, b []byte) error         // for sync publisher
	Load(node string, count int) (string, error) // load content for console log replay
	GetPublishChan() (chan []byte, error)        // for async publisher
	GetLoggerType() string                       // LineLogger or ByteLogger
	GetName() string                             // Identity of the publisher
}

type BasePublisher struct {
	name string
}

func (self *BasePublisher) GetName() string {
	return self.name
}

type NetworkPublisher struct {
	BasePublisher
	publisherChan chan []byte
}

func (self *NetworkPublisher) Publish(node string, b []byte) error {
	return common.ErrUnsupported
}

func (self *NetworkPublisher) Load(node string, count int) (string, error) {
	return "", common.ErrUnsupported
}

func (self *NetworkPublisher) GetLoggerType() string {
	return LINE_LOGGER
}

func (self *NetworkPublisher) GetPublishChan() (chan []byte, error) {
	return self.publisherChan, nil
}

func copyLast(last *[]byte, b []byte) {
	if len(b) == 0 {
		*last = nil
		return
	}
	*last = make([]byte, len(b))
	copy(*last, b)
}