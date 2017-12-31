package logger

import (
	"bytes"
	"encoding/json"
	"github.com/chenglch/goconserver/common"
	"time"
)

const (
	sendInterval = time.Second
)

var (
	LOGGER_INIT_MAP = map[string]func() (Logger, error){}
	plog            = common.GetLogger("github.com/chenglch/goconserver/console/logger")
	serverConfig    = common.GetServerConfig()
)

type Logger interface {
	Emit(node string, b []byte, last *[]byte) error // log the content from the target
	Prompt(node string, message string) error       // prompt message about console event
	Fetch(node string, count int) (string, error)   // to support console replay
}

func copyLast(last *[]byte, b []byte) {
	if len(b) == 0 {
		*last = nil
		return
	}
	*last = make([]byte, len(b))
	copy(*last, b)
}

type LineLogger struct {
	bufChans chan []byte
}

func (self *LineLogger) Emit(node string, b []byte, last *[]byte) error {
	var err error
	var buf []byte
	p := 0
	if *last != nil {
		b = append(*last, b...)
	}
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			var line bytes.Buffer
			line.Write(b[p:i])
			bufMap := make(map[string]string)
			bufMap["type"] = "console"
			bufMap["message"] = line.String()
			bufMap["node"] = node

			if serverConfig.Console.LogTimestamp {
				bufMap["date"] = time.Now().Format("2006-01-02 15:04:05.0000")
			}
			p = i + 1
			buf, err = json.Marshal(bufMap)
			if err != nil {
				plog.ErrorNode(node, err)
				continue
			}
			line.Reset()
			self.bufChans <- append(buf, []byte{'\n'}...)
		}
	}
	copyLast(last, b[p:])
	return nil
}

func (self *LineLogger) Fetch(node string, count int) (string, error) {
	return "", common.ErrLoggerType
}

func (self *LineLogger) Prompt(node string, message string) error {
	var err error
	var buf []byte
	bufMap := make(map[string]string)
	bufMap["type"] = "event"
	bufMap["message"] = message
	bufMap["node"] = node
	if serverConfig.Console.LogTimestamp {
		bufMap["data"] = time.Now().Format("2006-01-02 15:04:05.0000")
	}
	buf, err = json.Marshal(bufMap)
	if err != nil {
		plog.ErrorNode(node, err)
	}
	self.bufChans <- append(buf, []byte{'\n'}...)
	return nil
}
