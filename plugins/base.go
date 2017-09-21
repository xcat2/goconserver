package plugins

import (
	"errors"
	"fmt"
	"io"
)

const (
	SSH_DRIVER_TYPE = "ssh"
)

var (
	SUPPORTED_DRIVERS = map[string]bool{
		SSH_DRIVER_TYPE: true,
	}
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
	if driver == SSH_DRIVER_TYPE {
		consolePlugin, err = NewSSHConsole(name, params)
		if err != nil {
			plog.ErrorNode(name, fmt.Sprintf("Could not start ssh console for node %s.", name))
			return nil, err
		}
	}
	return consolePlugin, nil
}

func Validate(driver string, name string, params map[string]string) error {
	if driver == SSH_DRIVER_TYPE {
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
	}
	return errors.New(fmt.Sprintf("%s: Could not find %s driver", name, driver))
}
