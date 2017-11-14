package console

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/chenglch/goconserver/common"
	"golang.org/x/crypto/ssh/terminal"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

type ConsoleClient struct {
	common.Network
	host, port string
	origState  *terminal.State
	escape     int // client exit signal
	cr         int
	exit       chan bool
	retry      bool
	inputTask  *common.Task
	outputTask *common.Task
}

func NewConsoleClient(host string, port string) *ConsoleClient {
	return &ConsoleClient{host: host, port: port,
		exit:  make(chan bool, 0),
		retry: true}
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
	exit, pos := c.checkEscape(b, n)
	if exit == true {
		b = []byte(ExitSequence)
		n = len(b)
		c.retry = false
	}
	if pos >= n {
		return
	}
	c.SendByteWithLength(conn.(net.Conn), b[pos:n])
}

func (c *ConsoleClient) output(args ...interface{}) {
	b := args[0].([]interface{})[1].([]byte)
	conn := args[0].([]interface{})[0].(net.Conn)
	n, err := c.ReceiveInt(conn)
	if err != nil {
		fmt.Printf("\rCould not receive message, error: %s\r\n", err.Error())
		c.exit <- true
		return
	}
	b, err = c.ReceiveBytes(conn, n)
	if err != nil {
		fmt.Printf("\rCould not receive message, error: %s\r\n", err.Error())
		c.exit <- true
		return
	}
	b = c.transCr(b, n)
	n = len(b)
	for n > 0 {
		tmp, err := os.Stdout.Write(b)
		if err != nil {
			fmt.Printf("\rCould not send message, error: %s\r\n", err.Error())
			c.exit <- true
			return
		}
		n -= tmp
	}
}

func (c *ConsoleClient) contains(cmds []byte, cmd byte) bool {
	for _, v := range cmds {
		if v == cmd {
			return true
		}
	}
	return false
}

func (c *ConsoleClient) runClientCmd(cmd byte) {
	if cmd == CLIENT_CMD_HELP {
		fmt.Printf("Help message from congo:\r\n" +
			"Quit: Ctrl + E + C + .  \r\n" +
			"Help: Ctrl + E + C + ?  \r\b" +
			"The otherclient command is not supported temporarily.\r\n")
	}
}

func (c *ConsoleClient) checkEscape(b []byte, n int) (bool, int) {
	pos := 0
	for i := 0; i < n; i++ {
		ch := b[i]
		if c.escape == 0 {
			if ch == '\x05' {
				c.escape = 1
				pos = i + 1
			}
		} else if c.escape == 1 {
			if ch == 'c' {
				c.escape = 2
				pos = i + 1
			} else {
				c.escape = 0
			}
		} else if c.escape == 2 {
			if ch == CLIENT_CMD_EXIT {
				c.exit <- true
				return true, 0
			} else if c.contains(CLIENT_CMDS, ch) {
				c.runClientCmd(ch)
				c.escape = 0
				pos = i + 1
			} else {
				c.escape = 0
			}
		}
	}
	return false, pos
}

func (c *ConsoleClient) transCr(b []byte, n int) []byte {
	temp := make([]byte, 4096)
	j := 0
	for i := 0; i < n; i++ {
		ch := b[i]
		if c.cr == 0 {
			if ch == ' ' {
				c.cr = 1
			} else {
				temp[j] = ch
				j++
			}
		} else if c.cr == 1 {
			if ch == '\r' {
				c.cr = 2
			} else {
				temp[j], temp[j+1] = ' ', ch
				j += 2
				c.cr = 0
			}
		} else if c.cr == 2 {
			if ch == '\n' {
				temp[j], temp[j+1], temp[j+2] = ' ', '\r', ch
				j += 3
			} else {
				temp[j] = ch // ignore " \r"
				j++
			}
			c.cr = 0
		}
	}
	if c.cr == 1 {
		c.cr = 0
		temp[j] = ' '
		j++
	}
	return temp[0:j]
}

func (c *ConsoleClient) tryConnect(conn net.Conn, name string) (int, error) {
	m := make(map[string]string)
	m["name"] = name
	m["command"] = COMMAND_START_CONSOLE
	b, err := json.Marshal(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v", err)
		return STATUS_ERROR, err
	}
	consoleTimeout := time.Duration(clientConfig.ConsoleTimeout)
	err = c.SendByteWithLengthTimeout(conn, b, consoleTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %v", err)
		return STATUS_ERROR, err
	}
	status, err := c.ReceiveIntTimeout(conn, consoleTimeout)
	if err != nil {
		if err == io.EOF && status == STATUS_NOTFOUND {
			return STATUS_NOTFOUND, nil
		}
		fmt.Fprintf(os.Stderr, "Fatal error: %v", err)
		return STATUS_ERROR, err
	}
	return status, nil
}

func (c *ConsoleClient) Handle(conn net.Conn, name string) (string, error) {
	defer conn.Close()
	recvBuf := make([]byte, 4096)
	sendBuf := make([]byte, 4096)
	consoleTimeout := time.Duration(clientConfig.ConsoleTimeout)
	status, err := c.tryConnect(conn, name)
	if status == STATUS_REDIRECT {
		n, err := c.ReceiveIntTimeout(conn, consoleTimeout)
		if err != nil {
			return "", err
		}
		b, err := c.ReceiveBytesTimeout(conn, n, consoleTimeout)
		if err != nil && err != io.EOF {
			return "", err
		}
		plog.InfoNode(name, fmt.Sprintf("Redirect the node connecteion to %s", string(b)))
		return string(b), nil
	}
	if status != STATUS_CONNECTED {
		if status == STATUS_NOTFOUND {
			fmt.Printf("Could not find node %s \n", name)
			os.Exit(1)
		}
		plog.ErrorNode(name, fmt.Sprintf("Fatal error: Could not connect to %s\n", name))
		return "", common.ErrConnection
	}
	if !terminal.IsTerminal(int(os.Stdin.Fd())) {
		return "", common.ErrNotTerminal
	}
	c.origState, err = terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	defer terminal.Restore(int(os.Stdin.Fd()), c.origState)
	c.registerSignal()
	c.inputTask, err = common.GetTaskManager().RegisterLoop(c.input, conn, sendBuf)
	if err != nil {
		return "", err
	}
	defer common.GetTaskManager().Stop(c.inputTask.GetID())
	c.outputTask, err = common.GetTaskManager().RegisterLoop(c.output, conn, recvBuf)
	if err != nil {
		return "", err
	}
	defer common.GetTaskManager().Stop(c.outputTask.GetID())
	select {
	case <-c.exit:
		break
	}
	return "", nil
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
		tlsConfig, err := common.LoadClientTlsConfig(clientConfig.SSLCertFile, clientConfig.SSLKeyFile,
			clientConfig.SSLCACertFile, s.host)
		if err != nil {
			panic(err)
		}
		conn = tls.Client(conn, tlsConfig)
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
	httpClient := http.Client{Timeout: time.Second * 5}
	client := &common.HttpClient{Client: &httpClient, Headers: http.Header{}}
	if strings.HasPrefix(baseUrl, "https") && clientConfig.SSLKeyFile != "" &&
		clientConfig.SSLCertFile != "" && clientConfig.SSLCACertFile != "" {
		tlsConfig, err := common.LoadClientTlsConfig(clientConfig.SSLCertFile,
			clientConfig.SSLKeyFile, clientConfig.SSLCACertFile, clientConfig.ServerHost)
		if err != nil {
			panic(err)
		}
		client.Client.Transport = &http.Transport{TLSClientConfig: tlsConfig}
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
		return nodes, common.ErrInvalidType
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
