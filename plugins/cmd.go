package plugins

import (
	"fmt"
	"github.com/xcat2/goconserver/common"
	"github.com/kr/pty"
	"os"
	"os/exec"
	"strings"
)

const (
	DRIVER_CMD  = "cmd"
	DEFAULT_ENV = "CONSOLE_TYPE=gocons"
)

type CommondConsole struct {
	node    string // session name
	cmd     string
	params  []string
	envs    []string
	command *exec.Cmd
	pty     *os.File
}

func init() {
	DRIVER_INIT_MAP[DRIVER_CMD] = NewCommondConsole
	DRIVER_VALIDATE_MAP[DRIVER_CMD] = func(name string, params map[string]string) error {
		if _, ok := params["cmd"]; !ok {
			return common.NewErr(common.INVALID_PARAMETER, fmt.Sprintf("%s: Please specify the parameter cmd", name))
		}
		return nil
	}
}

func NewCommondConsole(node string, params map[string]string) (ConsolePlugin, error) {
	var env string
	var envs = []string{DEFAULT_ENV}
	cmd := params["cmd"]
	args := strings.Split(cmd, " ")
	// environment variables, format like "CONSOLE_TYPE=gocons DEBUG=true"
	if env, _ = params["env"]; env != "" {
		envs = append(envs, strings.Split(env, " ")...)
	}
	return &CommondConsole{
		node:   node,
		cmd:    args[0],
		params: args[1:],
		envs:   envs}, nil
}

func (self *CommondConsole) Start() (*BaseSession, error) {
	var err error
	self.command = exec.Command(self.cmd, self.params...)
	self.command.Env = append(os.Environ(), self.envs...)
	self.pty, err = pty.Start(self.command)
	if err != nil {
		plog.ErrorNode(self.node, err.Error())
		return nil, err
	}
	tty := common.Tty{}
	ttyWidth, ttyHeight, err := tty.GetSize(os.Stdin)
	if err != nil {
		plog.DebugNode(self.node, "Could not get tty size, use 80,80 as default")
		ttyHeight = 80
		ttyWidth = 80
	}
	if err = tty.SetSize(self.pty, ttyWidth, ttyHeight); err != nil {
		plog.ErrorNode(self.node, err.Error())
		return nil, err
	}
	return &BaseSession{In: self.pty, Out: self.pty, Session: self}, nil
}

func (self *CommondConsole) Close() error {
	self.pty.Close()
	return self.command.Process.Kill()
}

func (self *CommondConsole) Wait() error {
	return self.command.Wait()
}
