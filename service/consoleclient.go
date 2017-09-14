package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/chenglch/consoleserver/common"
	"github.com/chenglch/consoleserver/console"
	"golang.org/x/crypto/ssh/terminal"
)

func doSignal(handler common.SignalHandler) {
	s := common.SignalSetNew()
	s.Register(syscall.SIGINT, handler)
	s.Register(syscall.SIGTERM, handler)
	s.Register(syscall.SIGHUP, handler)

	for {
		c := make(chan os.Signal)
		var sigs []os.Signal
		for sig := range s.GetSigMap() {
			sigs = append(sigs, sig)
		}
		signal.Notify(c)
		sig := <-c

		err := s.Handle(sig, nil)
		if err != nil {
			fmt.Printf("unknown signal received: %v\n", sig)
			os.Exit(1)
		}
	}
}

type ConsoleClient struct {
	common.Network
	host, port string
	origState  *terminal.State
	escape     int // client exit signal
	exit       chan bool
	inputTask  *common.Task
	outputTask *common.Task
}

func NewConsoleClient(host string, port string) *ConsoleClient {
	return &ConsoleClient{host: host, port: port, exit: make(chan bool, 0)}
}

func (c *ConsoleClient) input(args ...interface{}) {
	b := args[0].([]interface{})[1].([]byte)
	conn := args[0].([]interface{})[0].(net.Conn)
	n, err := os.Stdin.Read(b)
	if err != nil {
		fmt.Println(err)
		c.exit <- true
		return
	}
	exit := c.checkEscape(b, n)
	if exit == -1 {
		b = []byte(console.ExitSequence)
		n = len(b)
	}
	c.SendByteWithLength(conn.(net.Conn), b[:n])
}

func (c *ConsoleClient) output(args ...interface{}) {
	b := args[0].([]interface{})[1].([]byte)
	conn := args[0].([]interface{})[0].(net.Conn)
	n, err := c.ReceiveInt(conn)
	if err != nil {
		fmt.Println(err)
		c.exit <- true
		return
	}
	b, err = c.ReceiveBytes(conn, n)
	if err != nil {
		fmt.Println(err)
		c.exit <- true
		return
	}
	n, err = os.Stdout.Write(b)
	if err != nil {
		fmt.Println(err)
		c.exit <- true
		return
	}
}

func (c *ConsoleClient) checkEscape(b []byte, n int) int {
	for i := 0; i < n; i++ {
		ch := b[i]
		if ch == '\x05' {
			c.escape = 1
		} else if ch == 'c' {
			if c.escape == 1 {
				c.escape = 2
			}
		} else if ch == '.' {
			if c.escape == 2 {
				c.exit <- true
				return -1
			}
		} else {
			c.escape = 0
		}
	}
	return 0
}

func (c *ConsoleClient) Handle(conn net.Conn, name string) error {
	m := make(map[string]string)
	m["name"] = name
	m["command"] = "start_console"
	b, err := json.Marshal(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		return err
	}
	err = c.SendByteWithLength(conn, b)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		return err
	}
	if !terminal.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "Fatal error: stdin is not terminal")
		return errors.New("stdin is not terminal")
	}
	c.origState, err = terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		return err
	}
	defer terminal.Restore(int(os.Stdin.Fd()), c.origState)
	handler := func(s os.Signal, arg interface{}) {
		fmt.Printf("handle signal: %v\n", s)
		terminal.Restore(int(os.Stdin.Fd()), c.origState)
		os.Exit(1)
	}
	go doSignal(handler)

	recvBuf := make([]byte, 4096)
	sendBuf := make([]byte, 4096)
	c.inputTask, err = common.GetTaskManager().RegisterLoop(c.input, conn, sendBuf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		return err
	}
	defer common.GetTaskManager().Stop(c.inputTask.GetID())
	c.outputTask, err = common.GetTaskManager().RegisterLoop(c.output, conn, recvBuf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		return err
	}
	defer common.GetTaskManager().Stop(c.outputTask.GetID())
	defer conn.Close()
	for {
		select {
		case exit := <-c.exit:
			if exit {
				return nil
			}
		default:
			runtime.Gosched()
		}
	}
	return nil
}

func (s *ConsoleClient) Connect() (net.Conn, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%s", s.host, s.port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
	return conn, nil
}
