package logger

import (
	"github.com/chenglch/goconserver/common"
	"sync"
)

func NewPipeline(loggerCfg *common.LoggerCfg) (*Pipeline, error) {
	pipeLine := new(Pipeline)
	pipeLine.rwLock = new(sync.RWMutex)
	pipeLine.mapping = make(map[string]Logger)
	pipeLine.loggers = make([]Logger, 0)
	// this lock may not be necessary
	pipeLine.rwLock.Lock()
	defer pipeLine.rwLock.Unlock()
	var err error
	var publisher Publisher
	i := 0
	for i = 0; i < len(loggerCfg.File); i++ {
		publisher, err = NewFilePublisher(&loggerCfg.File[i])
		if err != nil {
			plog.Error(err)
			return nil, err
		}
		pipeLine.register(publisher)
	}
	for i = 0; i < len(loggerCfg.TCP); i++ {
		publisher, err = NewTCPPublisher(&loggerCfg.TCP[i])
		if err != nil {
			plog.Error(err)
			return nil, err
		}
		pipeLine.register(publisher)
	}
	for i = 0; i < len(loggerCfg.UDP); i++ {
		publisher, err = NewUDPPublisher(&loggerCfg.UDP[i])
		if err != nil {
			plog.Error(err)
			return nil, err
		}
		pipeLine.register(publisher)
	}
	return pipeLine, nil
}

type Pipeline struct {
	rwLock   *sync.RWMutex
	mapping  map[string]Logger
	loggers  []Logger
	Periodic bool
}

func (self *Pipeline) register(publisher Publisher) {
	var logger Logger
	var ok bool
	if logger, ok = self.mapping[publisher.GetLoggerType()]; !ok {
		if publisher.GetLoggerType() == LINE_LOGGER {
			self.Periodic = true
		}
		logger = LOGGER_INIT_MAP[publisher.GetLoggerType()]()
		self.loggers = append(self.loggers, logger)
		self.mapping[publisher.GetLoggerType()] = logger
	}
	logger.Register(publisher)
}

// only LineLogger may affect the last pointer
func (self *Pipeline) MakeRecord(node string, b []byte, last *RemainBuffer) error {
	var err error
	for _, logger := range self.loggers {
		err = logger.MakeRecord(node, b, last)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *Pipeline) PromptLast(node string, last *RemainBuffer) error {
	var err error
	for _, logger := range self.loggers {
		err = logger.PromptLast(node, last)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *Pipeline) Fetch(node string, count int) (string, error) {
	var content string
	var err error
	for _, logger := range self.loggers {
		content, err = logger.Fetch(node, count)
		if err == nil {
			return content, nil
		}
	}
	return "", common.ErrLoggerType
}

func (self *Pipeline) Prompt(node string, message string) error {
	var err error
	for _, logger := range self.loggers {
		err = logger.Prompt(node, message)
		if err != nil {
			return err
		}
	}
	return nil
}
