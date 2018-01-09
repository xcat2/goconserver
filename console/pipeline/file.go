package logger

import (
	"fmt"
	"github.com/chenglch/goconserver/common"
	"os"
	"path/filepath"
)

const (
	FILE_PUBLISHER = "file"
)

func init() {
	PUBLISHER_INIT_MAP[FILE_PUBLISHER] = NewFilePublisher
}

func NewFilePublisher(params interface{}) (Publisher, error) {
	var fileCfg *common.FileCfg
	var ok bool
	if fileCfg, ok = params.(*common.FileCfg); !ok {
		return nil, common.ErrInvalidParameter
	}
	logDir := fileCfg.LogDir
	publisher := new(FilePublisher)
	publisher.name = fileCfg.Name
	if publisher.name == "" {
		publisher.name = UNKNOWN
	}
	publisher.fileCfg = fileCfg
	plog.Info(fmt.Sprintf("Starting FILE publisher: %s", publisher.name))
	if exist, _ := common.PathExists(logDir); !exist {
		err := os.MkdirAll(logDir, 0700)
		if err != nil {
			return nil, err
		}
	}
	return publisher, nil
}

type FilePublisher struct {
	BasePublisher
	fileCfg *common.FileCfg
}

func (self *FilePublisher) Publish(node string, b []byte) error {
	var err error
	path := fmt.Sprintf("%s%c%s.log", self.fileCfg.LogDir, filepath.Separator, node)
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

func (self *FilePublisher) Load(node string, count int) (string, error) {
	path := fmt.Sprintf("%s%c%s.log", self.fileCfg.LogDir, filepath.Separator, node)
	content, err := common.ReadTail(path, count)
	if err != nil {
		plog.ErrorNode(node, fmt.Sprintf("Could not read log file %s", path))
		return "", err
	}
	return content, nil
}

func (self *FilePublisher) GetPublishChan() (chan []byte, error) {
	return nil, common.ErrUnsupported
}

func (self *FilePublisher) GetLoggerType() string {
	return BYTE_LOGGER
}
