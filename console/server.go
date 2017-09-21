package console

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"

	"path"

	"errors"
	"github.com/chenglch/consoleserver/common"
	"github.com/chenglch/consoleserver/plugins"
	"time"
)

const (
	STATUS_AVAIABLE = iota
	STATUS_ENROLL
	STATUS_CONNECTED
	STATUS_ERROR
)

var (
	plog           = common.GetLogger("github.com/chenglch/consoleserver/service")
	nodeManager    *NodeManager
	serverConfig   = common.GetServerConfig()
	nodeConfigFile string
	STATUS_MAP     = map[int]string{
		STATUS_AVAIABLE:  "avaiable",
		STATUS_ENROLL:    "enroll",
		STATUS_CONNECTED: "connected",
		STATUS_ERROR:     "error",
	}
)

type Node struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver"` // node type cmd, ssh, ipmitool
	Params   map[string]string `json:"params"`
	Ondemand bool              `json:"ondemand, true"`
	State    string            // string value of status
	status   int
	console  *Console
	ready    chan bool // indicate session has been established with remote
}

func NewNode() *Node {
	node := new(Node)
	node.ready = make(chan bool, 0) // block client
	node.status = STATUS_AVAIABLE
	node.Ondemand = true
	return node
}

func (node *Node) Validate() error {
	if _, ok := plugins.SUPPORTED_DRIVERS[node.Driver]; !ok {
		return errors.New(fmt.Sprintf("Could find driver %s in the supported dictionary", node.Driver))
	}
	if err := plugins.Validate(node.Driver, node.Name, node.Params); err != nil {
		return err
	}
	return nil
}

func (node *Node) StartConsole() {
	var consolePlugin plugins.ConsolePlugin
	var err error
	var baseSession *plugins.BaseSession
	consolePlugin, err = plugins.StartConsole(node.Driver, node.Name, node.Params)
	if err != nil {
		node.status = STATUS_ERROR
		return
	}
	baseSession, err = consolePlugin.Start()
	if err != nil {
		node.status = STATUS_ERROR
		return
	}
	console := NewConsole(baseSession, node)
	node.console = console
	console.Start()
}

func (node *Node) StopConsole() {
	if node.console == nil {
		plog.WarningNode(node.Name, "Console is not started.")
		return
	}
	node.console.Stop()
	node.status = STATUS_AVAIABLE
}

func (node *Node) SetStatus(status int) {
	node.status = status
}

func (node *Node) GetStatus() int {
	return node.status
}

func (node *Node) GetReadyChan() chan bool {
	return node.ready
}

type ConsoleServer struct {
	common.Network
	host, port string
}

func NewConsoleServer(host string, port string) *ConsoleServer {
	nodeManager = GetNodeManager()
	return &ConsoleServer{host: host, port: port}
}

func (c *ConsoleServer) handle(conn interface{}) {
	plog.Debug("New client connection received.")
	err := conn.(*net.TCPConn).SetKeepAlive(true)
	if err != nil {
		plog.Error(fmt.Sprintf("Cloud not make connection keepalive %s", err.Error()))
		return
	}
	err = conn.(*net.TCPConn).SetKeepAlivePeriod(30 * time.Second)
	if err != nil {
		plog.Error(fmt.Sprintf("Cloud not make connection keepalive %s", err.Error()))
		return
	}
	socketTimeout := time.Duration(serverConfig.Console.SocketTimeout)
	size, err := c.ReceiveIntTimeout(conn.(net.Conn), socketTimeout)
	if err != nil {
		return
	}
	b, err := c.ReceiveBytesTimeout(conn.(net.Conn), size, socketTimeout)
	if err != nil {
		return
	}
	data := make(map[string]string)
	if err := json.Unmarshal(b, &data); err != nil {
		plog.Error(err)
		return
	}
	if _, ok := nodeManager.Nodes[data["name"]]; !ok {
		plog.ErrorNode(data["name"], "Could not find this node.")
		c.SendInt(conn.(net.Conn), STATUS_ERROR)
		return
	}
	node := nodeManager.Nodes[data["name"]]
	if data["command"] == "start_console" {
		if node.status != STATUS_CONNECTED {
			common.GetTaskManager().Register(node.StartConsole)
			if err := common.TimeoutChan(node.ready, serverConfig.Console.TargetTimeout); err != nil {
				plog.ErrorNode(node.Name, err)
				return
			}
		}
		plog.InfoNode(node.Name, "Register client connection successfully.")
		// reply success message to the client
		c.SendInt(conn.(net.Conn), STATUS_CONNECTED)
		node.console.Accept(conn.(net.Conn))
	}
}

func (s *ConsoleServer) Listen() {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%s", s.host, s.port))
	if err != nil {
		plog.Error(err)
		return
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			plog.Error(err)
			return
		}
		common.GetTaskManager().Register(s.handle, conn)
	}
}

type NodeManager struct {
	Nodes map[string]*Node
	wgMu  sync.RWMutex
}

func GetNodeManager() *NodeManager {
	if nodeManager != nil {
		return nodeManager
	}
	nodeManager = new(NodeManager)
	nodeManager.Nodes = make(map[string]*Node)
	nodeConfigFile = path.Join(serverConfig.Console.DataDir, "nodes.json")
	if ok, _ := common.PathExists(nodeConfigFile); ok {
		bytes, err := ioutil.ReadFile(nodeConfigFile)
		if err != nil {
			panic(err)
		}
		if err := json.Unmarshal(bytes, &nodeManager.Nodes); err != nil {
			panic(err)
		}
		for _, v := range nodeManager.Nodes {
			v.SetStatus(STATUS_AVAIABLE)
			v.ready = make(chan bool, 0)
			if v.Ondemand == false {
				common.GetTaskManager().Register(v.StartConsole)
			}
		}
	}
	consoleServer := NewConsoleServer(serverConfig.Global.Host, serverConfig.Console.Port)
	common.GetTaskManager().Register(consoleServer.Listen)
	return nodeManager
}

func (m *NodeManager) Save(w http.ResponseWriter, req *http.Request) error {
	var data []byte
	var err error
	if data, err = json.Marshal(m.Nodes); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return err
	}
	m.wgMu.Lock()
	err = common.WriteJsonFile(nodeConfigFile, data)
	m.wgMu.Unlock()
	if err != nil {
		plog.Error(err)
		return err
	}
	return nil
}
