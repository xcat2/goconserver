package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/chenglch/goconserver/common"
	"strings"
	"time"
)

const (
	EVENT_TYPE   = "event"   // some event like connected or disconnected
	CONSOLE_TYPE = "console" // console message
	REMAIN_TYPE  = "remain"  // last incomplete record
)

func init() {
	LOGGER_INIT_MAP[LINE_LOGGER] = NewLineLogger
	LOGGER_INIT_MAP[BYTE_LOGGER] = NewByteLogger
}

func NewLineBuf(Type string, message string, node string, LogTimestamp bool) *LineBuf {
	buf := &LineBuf{Type: Type, Message: message, Node: node}
	if LogTimestamp {
		buf.Date = time.Now().Format(serverConfig.Console.TimeFormat)
	}
	return buf
}

type LineBuf struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Node    string `json:"node"`
	Date    string `json:"date,omitempty"`
}

func (self *LineBuf) Marshal() ([]byte, error) {
	buf, err := json.Marshal(self)
	if err != nil {
		return nil, err
	}
	return buf, nil
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

func (self *LineLogger) getEndPos(b []byte, start, end int) int {
	for end > start {
		if b[end-1] != '\r' {
			break
		}
		end--
	}
	return end
}

func (self *LineLogger) MakeRecord(node string, b []byte, last *RemainBuffer) error {
	var err error
	var buf []byte
	p := 0
	if last.Buf != nil {
		b = append(last.Buf, b...)
	}
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			end := self.getEndPos(b, p, i)
			lineBuf := NewLineBuf(CONSOLE_TYPE, string(b[p:end]), node, serverConfig.Console.LogTimestamp)
			p = i + 1
			buf, err = lineBuf.Marshal()
			if err != nil {
				plog.ErrorNode(node, err)
				continue
			}
			self.bufChan <- append(buf, []byte{'\n'}...)
		}
	}
	if len(b[p:]) >= 4096 {
		lineBuf := NewLineBuf(CONSOLE_TYPE, string(b[p:]), node, serverConfig.Console.LogTimestamp)
		buf, err = lineBuf.Marshal()
		if err != nil {
			plog.ErrorNode(node, err)
		}
		self.bufChan <- append(buf, []byte{'\n'}...)
		p = len(b)
	}
	copyLast(last, b[p:])
	return nil
}

func (self *LineLogger) PromptLast(node string, last *RemainBuffer) error {
	var err error
	var buf []byte
	if last.Buf != nil {
		message := string(last.Buf)
		if message == "" {
			return nil
		}
		lineBuf := NewLineBuf(REMAIN_TYPE, message, node, serverConfig.Console.LogTimestamp)
		buf, err = lineBuf.Marshal()
		if err != nil {
			plog.ErrorNode(node, err)
			return err
		}
		self.bufChan <- append(buf, []byte{'\n'}...)
	}
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
		case <-time.After(common.PIPELINE_SEND_INTERVAL * 2):
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
	lineBuf := NewLineBuf(EVENT_TYPE, message, node, serverConfig.Console.LogTimestamp)
	buf, err = lineBuf.Marshal()
	if err != nil {
		plog.ErrorNode(node, err)
		return err
	}
	self.bufChan <- append(buf, []byte{'\n'}...)
	return nil
}

func (self *LineLogger) run() {
	buf := new(bytes.Buffer)
	plog.Debug("Starting line logger")
	ticker := time.NewTicker(common.PIPELINE_SEND_INTERVAL)
	// timeout may block data
	for {
		select {
		case b := <-self.bufChan:
			buf.Write(b)
			if buf.Len() > 8192 {
				// too large, send immediately
				self.emit(buf.Bytes())
				// slice in channel is transferred by reference
				buf = new(bytes.Buffer)
			}
		case <-ticker.C:
			if buf.Len() > 0 {
				// send buf to the channel
				self.emit(buf.Bytes())
				buf = new(bytes.Buffer)
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

func (self *ByteLogger) MakeRecord(node string, b []byte, last *RemainBuffer) error {
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

func (self *ByteLogger) PromptLast(node string, last *RemainBuffer) error {
	return nil
}
