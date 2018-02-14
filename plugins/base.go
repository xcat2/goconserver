package plugins

import (
	"fmt"
	"github.com/xcat2/goconserver/common"
	"io"
)

var (
	DRIVER_INIT_MAP     = map[string]func(string, map[string]string) (ConsolePlugin, error){}
	DRIVER_VALIDATE_MAP = map[string]func(string, map[string]string) error{}
	plog                = common.GetLogger("github.com/xcat2/goconserver/plugins")
)

type ConsoleSession interface {
	Wait() error
	Close() error
	Start() (*BaseSession, error)
}

type BaseSession struct {
	In      io.Writer
	Out     io.Reader
	Session ConsoleSession
}

type ConsolePlugin interface {
	Start() (*BaseSession, error)
}

func StartConsole(driver string, name string, params map[string]string) (ConsolePlugin, error) {
	var consolePlugin ConsolePlugin
	var err error
	consolePlugin, err = DRIVER_INIT_MAP[driver](name, params)
	if err != nil {
		plog.ErrorNode(name, fmt.Sprintf("Could not start %s console for node %s.", driver, name))
		return nil, err
	}
	return consolePlugin, nil
}

func Validate(driver string, name string, params map[string]string) error {
	if fc, ok := DRIVER_VALIDATE_MAP[driver]; ok {
		return fc(name, params)
	}
	plog.ErrorNode(name, fmt.Sprintf("Could not find driver %s", driver))
	return common.ErrDriverNotExist
}
