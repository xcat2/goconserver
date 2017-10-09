package plugins

import (
	"github.com/chenglch/consoleserver/common"
	"github.com/kr/pty"
	"os"
	"os/exec"
	"strings"
)

type CommondConsole struct {
	node    string // session name
	cmd     string
	params  []string
	command *exec.Cmd
}

func NewCommondConsole(node string, params map[string]string) *CommondConsole {
	cmd := params["cmd"]
	args := strings.Split(cmd, " ")
	return &CommondConsole{node: node, cmd: args[0], params: args[1:]}
}

func (c *CommondConsole) Start() (*BaseSession, error) {
	c.command = exec.Command(c.cmd, c.params...)
	pty, err := pty.Start(c.command)
	if err != nil {
		return nil, err
	}
	tty := common.Tty{}
	ttyWidth, ttyHeight, err := tty.GetSize(os.Stdin)
	if err != nil {
		plog.WarningNode(c.node, "Could not get tty size, use 80,80 as default")
		ttyHeight = 80
		ttyWidth = 80
	}
	if err = tty.SetSize(pty, ttyWidth, ttyHeight); err != nil {
		return nil, err
	}
	return &BaseSession{In: pty, Out: pty, Session: c}, nil
}

func (c *CommondConsole) Close() error {
	return c.command.Process.Kill()

}

func (c *CommondConsole) Wait() error {
	return c.command.Wait()
}
