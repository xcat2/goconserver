package logger

import (
	"bytes"
	"fmt"
	"github.com/chenglch/goconserver/common"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	FILE_LOGGER = "file_logger"
)

func init() {
	LOGGER_INIT_MAP[FILE_LOGGER] = NewFileLogger
}

func NewFileLogger() (Logger, error) {
	plog.Debug("Starting file logger")
	if exist, _ := common.PathExists(serverConfig.Console.FileLogger.LogDir); !exist {
		err := os.MkdirAll(serverConfig.Console.FileLogger.LogDir, 0700)
		if err != nil {
			return nil, err
		}
	}
	return &FileLogger{}, nil
}

type FileLogger struct{}

func (self *FileLogger) insertStamp(b []byte) ([]byte, error) {
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

func (self *FileLogger) logger(path string, b []byte) error {
	var err error
	fd, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()
	l := len(b)
	for l > 0 {
		n, err := fd.Write(b)
		if err != nil {
			return err
		}
		l -= n
	}
	return nil
}

func (self *FileLogger) Emit(node string, b []byte, last *[]byte) error {
	// FileLogger do not left the buffer that has not been emited
	var err error
	if serverConfig.Console.LogTimestamp {
		b, err = self.insertStamp(b)
		if err != nil {
			plog.ErrorNode(node, err)
			return err
		}
	}
	path := fmt.Sprintf("%s%c%s.log", serverConfig.Console.FileLogger.LogDir, filepath.Separator, node)
	err = self.logger(path, b)
	if err != nil {
		plog.ErrorNode(node, err)
		return err
	}
	return nil
}

func (self *FileLogger) Fetch(node string, count int) (string, error) {
	path := fmt.Sprintf("%s%c%s.log", serverConfig.Console.FileLogger.LogDir, filepath.Separator, node)
	content, err := common.ReadTail(path, count)
	if err != nil {
		plog.ErrorNode(node, fmt.Sprintf("Could not read log file %s", path))
		return "", err
	}
	return content, nil
}

func (self *FileLogger) Prompt(node string, message string) error {
	var err error
	path := fmt.Sprintf("%s%c%s.log", serverConfig.Console.FileLogger.LogDir, filepath.Separator, node)
	if !strings.HasSuffix(message, "\r\n") {
		message = message + "\r\n"
	}
	if serverConfig.Console.LogTimestamp {
		message = "\r\n[" + time.Now().Format("2006-01-02 15:04:05") + "] " + message
	}
	err = self.logger(path, []byte(message))
	if err != nil {
		plog.ErrorNode(node, err)
		return err
	}
	return nil
}
