package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/chenglch/goconserver/common"
	"strings"
	"time"
)

func init() {
	LOGGER_INIT_MAP[LINE_LOGGER] = NewLineLogger
	LOGGER_INIT_MAP[BYTE_LOGGER] = NewByteLogger
}

func NewLineLogger() Logger {
	logger := LineLogger{
		publishers: make([]Publisher, 0),
		bufChan:    make(chan []byte, serverConfig.Global.Worker),
	}
	go logger.run()
	return &logger
}

type LineLogger struct {
	publishers []Publisher
	bufChan    chan []byte
}

func (self *LineLogger) Register(publisher Publisher) {
	self.publishers = append(self.publishers, publisher)
}

func (self *LineLogger) MakeRecord(node string, b []byte, last *[]byte) error {
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
				bufMap["date"] = time.Now().Format("2006-01-02 15:04:05.00000")
			}
			p = i + 1
			buf, err = json.Marshal(bufMap)
			if err != nil {
				plog.ErrorNode(node, err)
				continue
			}
			line.Reset()
			self.bufChan <- append(buf, []byte{'\n'}...)
		}
	}
	copyLast(last, b[p:])
	return nil
}

func (self *LineLogger) emit(buf []byte) {
	for _, publisher := range self.publishers {
		publishChan, err := publisher.GetPublishChan()
		if err == common.ErrUnsupported {
			continue
		}
		if err != nil {
			plog.Error(fmt.Sprintf("Could not get the publish channel, Error: ", err))
			continue
		}
		select {
		case publishChan <- buf:
		case <-time.After(sendInterval * 2):
			plog.Warn(fmt.Sprintf("Timeout for waiting publisher %s", publisher.GetName()))
		}
	}
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
		bufMap["date"] = time.Now().Format("2006-01-02 15:04:05.00000")
	}
	buf, err = json.Marshal(bufMap)
	if err != nil {
		plog.ErrorNode(node, err)
		return err
	}
	self.bufChan <- append(buf, []byte{'\n'}...)
	return nil
}

func (self *LineLogger) run() {
	var buf bytes.Buffer
	plog.Debug("Starting line logger")
	ticker := time.NewTicker(sendInterval)
	// timeout may block data
	for {
		select {
		case b := <-self.bufChan:
			buf.Write(b)
		case <-ticker.C:
			if buf.Len() > 0 {
				// send buf to the channel
				self.emit(buf.Bytes())
				buf.Reset()
			}
		}
	}
}

func NewByteLogger() Logger {
	return &ByteLogger{publishers: make([]Publisher, 0)}
}

type ByteLogger struct {
	publishers []Publisher
}

func (self *ByteLogger) Register(publisher Publisher) {
	self.publishers = append(self.publishers, publisher)
}

func (self *ByteLogger) insertStamp(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	p := 0
	defer func() {
		// butes.Buffer may panic with ErrTooLarge
		if r := recover(); r != nil {
			plog.Error(r)
		}
	}()
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			buf.Write(b[p:i]) // ignore retnru value as err is always nil
			buf.WriteString("\n[" + time.Now().Format("2006-01-02 15:04:05") + "] ")
			p = i + 1
		}
	}
	if p != len(b) {
		buf.Write(b[p:])
	}
	return buf.Bytes(), nil
}

func (self *ByteLogger) MakeRecord(node string, b []byte, last *[]byte) error {
	// FileLogger do not left the buffer that has not been emited
	var err error
	if serverConfig.Console.LogTimestamp {
		b, err = self.insertStamp(b)
		if err != nil {
			plog.ErrorNode(node, err)
			return err
		}
	}
	for _, publisher := range self.publishers {
		err = publisher.Publish(node, b)
		if err != nil {
			plog.ErrorNode(node, err)
			continue
		}
	}
	return nil
}

func (self *ByteLogger) Fetch(node string, count int) (string, error) {
	var err error
	var content string
	for _, publisher := range self.publishers {
		content, err = publisher.Load(node, count)
		if err != nil {
			continue
		}
		// Got the content, stop trying
		break
	}
	if err != nil {
		plog.ErrorNode(node, err)
		return "", err
	}
	return content, nil
}

func (self *ByteLogger) Prompt(node string, message string) error {
	var err error
	if !strings.HasSuffix(message, "\r\n") {
		message = message + "\r\n"
	}
	if serverConfig.Console.LogTimestamp {
		message = "\r\n[" + time.Now().Format("2006-01-02 15:04:05") + "] " + message
	}
	for _, publisher := range self.publishers {
		err = publisher.Publish(node, []byte(message))
		if err != nil {
			plog.ErrorNode(node, err)
			continue
		}
	}
	return nil
}
