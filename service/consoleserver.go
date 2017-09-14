package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"

	"github.com/chenglch/consoleserver/common"
	"github.com/chenglch/consoleserver/console"
	"github.com/chenglch/consoleserver/plugins"
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
	plog             *common.Logger
	nodeConfigFile   = "config/nodes.json"
	globalConfigFile = "consoleserver.json"
	nodeManager      *NodeManager
)

func init() {
	plog = common.GetLogger("github.com/chenglch/consoleserver/service/node")
}

type Node struct {
	Name    string            `json:"name"`
	Driver  string            `json:"driver"` // node type cmd, ssh, ipmitool
	Params  map[string]string `json:params`
	status  int
	console *console.Console
}

func (node *Node) StartConsole() {
	if node.Driver == SSH_DRIVER_TYPE {
		node.startSSHConsole()
	}
}

func (node *Node) SetStatus(status int) {
	node.status = status
}

func (node *Node) GetStatus() int {
	return node.status
}

func (node *Node) startSSHConsole() {
	var password, privateKey, port string

	if _, ok := node.Params["host"]; !ok {
		node.status = STATUS_ERROR
		plog.ErrorNode(node.Name, "host parameter is not defined")
		return
	}
	host := node.Params["host"]
	if _, ok := node.Params["port"]; !ok {
		port = "22"
	} else {
		port = node.Params["port"]
	}
	if _, ok := node.Params["user"]; !ok {
		node.status = STATUS_ERROR
		plog.ErrorNode(node.Name, "user parameter is not defined")
		return
	}
	user := node.Params["user"]
	if _, ok := node.Params["password"]; ok {
		password = node.Params["password"]
	}
	if _, ok := node.Params["private_key"]; ok {
		privateKey = node.Params["privatekey"]
	}
	if privateKey == "" && password == "" {
		node.status = STATUS_ERROR
		plog.ErrorNode(node.Name, "private_key and password, at least one of the parameter should be specified")
		return
	}
	sshConsole, err := plugins.NewSSHConsole(host, port, user, password, privateKey, node.Name)
	if err != nil {
		plog.ErrorNode(node.Name, err.Error())
		node.status = STATUS_ERROR
		return
	}
	console, err := sshConsole.Start()
	if err != nil {
		plog.ErrorNode(node.Name, err.Error())
		node.status = STATUS_ERROR
	}
	node.console = console
	console.Start()
}

type ConsoleServer struct {
	common.Network
	host, port string
}

func NewConsoleServer(host string, port string) *ConsoleServer {
	return &ConsoleServer{host: host, port: port}
}

func (c *ConsoleServer) handle(conn interface{}) {
	size, err := c.ReceiveInt(conn.(net.Conn))
	if err != nil {
		return
	}
	b, err := c.ReceiveBytes(conn.(net.Conn), size)
	if err != nil {
		return
	}
	data := make(map[string]string)
	if err := json.Unmarshal(b, &data); err != nil {
		plog.Error(err)
		return
	}
	if _, ok := nodeManager.NodeMap[data["name"]]; !ok {
		plog.ErrorNode(data["name"], "Could not find this node.")
		return
	}
	node := nodeManager.Nodes[nodeManager.NodeMap[data["name"]]]
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
	Nodes   []*Node
	NodeMap map[string]int
	wgMu    sync.RWMutex
}

func NewNodeManager() *NodeManager {
	if nodeManager != nil {
		return nodeManager
	}
	nodeManager = new(NodeManager)
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
		nodeManager.RefreshNodeMap()
	}
	consoleServer := NewConsoleServer("127.0.0.1", "12345")
	common.GetTaskManager().Register(consoleServer.Listen)
	return nodeManager
}

func (m *NodeManager) RefreshNodeMap() {
	if m == nil {
		return
	}
	m.NodeMap = make(map[string]int)
	for i, v := range m.Nodes {
		m.NodeMap[v.Name] = i
	}
}

func (m *NodeManager) Save(w http.ResponseWriter, req *http.Request) error {
	var data []byte
	var err error
	if data, err = json.Marshal(m.Nodes); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return err
	}
	m.wgMu.Lock()
	common.WriteJsonFile(nodeConfigFile, data)
	m.wgMu.Unlock()
	return nil
}
