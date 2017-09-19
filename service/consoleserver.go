package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"

	"path"

	"github.com/chenglch/consoleserver/common"
	"github.com/chenglch/consoleserver/console"
	"github.com/chenglch/consoleserver/plugins"
	"time"
)

const (
	SSH_DRIVER_TYPE = "ssh"
	STATUS_INIT     = iota
	STATUS_ENROLL
	STATUS_CONNECTED
	STATUS_ERROR
	STATUS_NOT_SUPPORTED
)

var (
	plog           = common.GetLogger("github.com/chenglch/consoleserver/service/node")
	nodeManager    *NodeManager
	serverConfig   *common.ServerConfig
	nodeConfigFile string
)

func init() {
	serverConfig = common.GetServerConfig()
}

type ConsolePlugin interface {
	Start() (*console.Console, error)
}

type Node struct {
	Name    string            `json:"name"`
	Driver  string            `json:"driver"` // node type cmd, ssh, ipmitool
	Params  map[string]string `json:params`
	status  int
	console *console.Console
}

func (node *Node) StartConsole() {
	var consolePlugin ConsolePlugin
	var err error
	var console *console.Console
	if node.Driver == SSH_DRIVER_TYPE {
		consolePlugin, err = plugins.NewSSHConsole(node.Name, node.Params)
	}
	console, err = consolePlugin.Start()
	if err != nil {
		node.status = STATUS_ERROR
	}
	node.console = console
	console.Start()
}

func (node *Node) StopConsole() {
	if node.console == nil {
		plog.ErrorNode(node.Name, "Console is not started.")
	}
	node.console.Stop()
}

func (node *Node) SetStatus(status int) {
	node.status = status
}

func (node *Node) GetStatus() int {
	return node.status
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
	if node.console == nil {
		plog.ErrorNode(node.Name, "The console session to the remote target is not started, refuse this connection from client.")
		c.SendInt(conn.(net.Conn), STATUS_ERROR)
		return
	}
	// reply success message to the client
	c.SendInt(conn.(net.Conn), STATUS_CONNECTED)
	if data["command"] == "start_console" {
		plog.InfoNode(node.Name, "Register client connection successfully.")
		node.console.AddConn(conn.(net.Conn))
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
			v.SetStatus(STATUS_INIT)
			common.GetTaskManager().Register(v.StartConsole)
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
