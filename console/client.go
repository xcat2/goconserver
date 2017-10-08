package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"crypto/tls"
	"github.com/chenglch/consoleserver/common"
	"golang.org/x/crypto/ssh/terminal"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

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
		b = []byte(ExitSequence)
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
		fmt.Fprintf(os.Stderr, "Fatal error: %v", err)
		return err
	}
	consoleTimeout := time.Duration(clientConfig.ConsoleTimeout)
	err = c.SendByteWithLengthTimeout(conn, b, consoleTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v", err)
		return err
	}
	status, err := c.ReceiveIntTimeout(conn, consoleTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v", err)
		return err
	}
	if status != STATUS_CONNECTED {
		fmt.Fprintf(os.Stderr, "Fatal error: Could not connect to %s\n", name)
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
	c.registerSignal()
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

	select {
	case <-c.exit:
		break
	}
	return nil
}

func (s *ConsoleClient) Connect() (net.Conn, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%s", s.host, s.port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
	clientConfig := common.GetClientConfig()
	clientTimeout := time.Duration(clientConfig.ConsoleTimeout)
	conn, err := net.DialTimeout("tcp", tcpAddr.String(), clientTimeout*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
	err = conn.(*net.TCPConn).SetKeepAlive(true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cloud not make connection keepalive %s\n", err.Error())
		os.Exit(1)
	}
	err = conn.(*net.TCPConn).SetKeepAlivePeriod(30 * time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cloud not make connection keepalive %s\n", err.Error())
		os.Exit(1)
	}
	if clientConfig.SSLCertFile != "" && clientConfig.SSLKeyFile != "" && clientConfig.SSLCACertFile != "" {
		conn = tls.Client(conn, common.LoadClientTlsConfig(clientConfig.SSLCertFile, clientConfig.SSLKeyFile,
			clientConfig.SSLCACertFile, clientConfig.ServerHost))
		err = conn.(*tls.Conn).Handshake()
		if err != nil {
			return nil, err
		}
	}
	return conn, nil
}

func (c *ConsoleClient) registerSignal() {
	exitHandler := func(s os.Signal, arg interface{}) {
		fmt.Fprintf(os.Stderr, "handle signal: %v\n", s)
		terminal.Restore(int(os.Stdin.Fd()), c.origState)
		os.Exit(1)
	}
	signalSet := common.GetSignalSet()
	signalSet.Register(syscall.SIGINT, exitHandler)
	signalSet.Register(syscall.SIGTERM, exitHandler)
	signalSet.Register(syscall.SIGHUP, exitHandler)
	windowSizeHandler := func(s os.Signal, arg interface{}) {}
	signalSet.Register(syscall.SIGWINCH, windowSizeHandler)
	go common.DoSignal()
}

type CongoClient struct {
	client  *common.HttpClient
	baseUrl string
}

func NewCongoClient(baseUrl string) *CongoClient {
	baseUrl = strings.TrimSuffix(baseUrl, "/")
	clientConfig := common.GetClientConfig()
	client := &common.HttpClient{Client: http.DefaultClient, Headers: http.Header{}}
	if strings.HasPrefix(baseUrl, "https") && clientConfig.SSLKeyFile != "" &&
		clientConfig.SSLCertFile != "" && clientConfig.SSLCACertFile != "" {
		client.Client.Transport = &http.Transport{TLSClientConfig: common.LoadClientTlsConfig(clientConfig.SSLCertFile,
			clientConfig.SSLKeyFile, clientConfig.SSLCACertFile, clientConfig.ServerHost)}
	}
	return &CongoClient{client: client, baseUrl: baseUrl}
}

func (c *CongoClient) List() ([]interface{}, error) {
	url := fmt.Sprintf("%s/nodes", c.baseUrl)
	var nodes []interface{}
	ret, err := c.client.Get(url, nil, nil, false)
	if err != nil {
		return nodes, err
	}
	val, ok := ret.(map[string]interface{})["nodes"].([]interface{})
	if !ok {
		return nodes, errors.New("The data received from server is not correct.")
	}
	return val, nil
}

func (c *CongoClient) Show(node string) (interface{}, error) {
	url := fmt.Sprintf("%s/nodes/%s", c.baseUrl, node)
	var ret interface{}
	ret, err := c.client.Get(url, nil, nil, true)
	if err != nil {
		return ret, err
	}
	return ret, nil
}

func (c *CongoClient) Logging(node string, state string) (interface{}, error) {
	url := fmt.Sprintf("%s/nodes/%s", c.baseUrl, node)
	params := neturl.Values{}
	params.Set("state", state)
	var ret interface{}
	ret, err := c.client.Put(url, &params, nil, false)
	if err != nil {
		return ret, err
	}
	return ret, nil
}

func (c *CongoClient) Delete(node string) (interface{}, error) {
	url := fmt.Sprintf("%s/nodes/%s", c.baseUrl, node)
	var ret interface{}
	ret, err := c.client.Delete(url, nil, nil, false)
	if err != nil {
		return ret, err
	}
	return ret, nil
}

func (c *CongoClient) Create(node string, attribs map[string]interface{}, params map[string]interface{}) (interface{}, error) {
	url := fmt.Sprintf("%s/nodes", c.baseUrl)
	data := attribs
	data["params"] = params
	data["name"] = node
	var ret interface{}
	ret, err := c.client.Post(url, nil, data, false)
	if err != nil {
		return ret, err
	}
	return ret, nil
}
