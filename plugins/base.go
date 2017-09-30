package plugins

import (
	"errors"
	"fmt"
	"github.com/chenglch/consoleserver/common"
	"io"
)

const (
	DRIVER_SSH = "ssh"
	DRIVER_CMD = "cmd"
)

var (
	SUPPORTED_DRIVERS = map[string]bool{
		DRIVER_SSH: true,
		DRIVER_CMD: true,
	}
	plog = common.GetLogger("github.com/chenglch/consoleserver/plugins")
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
	if driver == DRIVER_SSH {
		consolePlugin, err = NewSSHConsole(name, params)
		if err != nil {
			plog.ErrorNode(name, fmt.Sprintf("Could not start ssh console for node %s.", name))
			return nil, err
		}
	} else if driver == DRIVER_CMD {
		consolePlugin = NewCommondConsole(name, params)
	}
	return consolePlugin, nil
}

func Validate(driver string, name string, params map[string]string) error {
	if driver == DRIVER_SSH {
		if _, ok := params["host"]; !ok {
			return errors.New(fmt.Sprintf("node %s: Parameter host is not defined", name))
		}
		if _, ok := params["user"]; !ok {
			return errors.New(fmt.Sprintf("node %s: Parameter user is not defined", name))
		}
		_, ok1 := params["password"]
		_, ok2 := params["private_key"]
		if !ok1 && !ok2 {
			return errors.New(fmt.Sprintf("node %s: At least one of the parameter within private_key and password should be specified", name))
		}
		return nil
	} else if driver == DRIVER_CMD {
		if _, ok := params["cmd"]; !ok {
			return errors.New(fmt.Sprintf("node %s: Please specify the command", name))
		}
		return nil
	}
	return errors.New(fmt.Sprintf("%s: Could not find %s driver", name, driver))
}
