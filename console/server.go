package console

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chenglch/goconserver/common"
	pb "github.com/chenglch/goconserver/console/consolepb"
	"github.com/chenglch/goconserver/plugins"
	"github.com/chenglch/goconserver/storage"

	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
)

const (
	STATUS_AVAIABLE = iota
	STATUS_ENROLL
	STATUS_CONNECTED
	STATUS_ERROR
	STATUS_REDIRECT

	CONSOLE_ON            = "on"
	CONSOLE_OFF           = "off"
	COMMAND_START_CONSOLE = "start_console"
)

var (
	plog         = common.GetLogger("github.com/chenglch/goconserver/service")
	nodeManager  *NodeManager
	serverConfig = common.GetServerConfig()
	STATUS_MAP   = map[int]string{
		STATUS_AVAIABLE:  "avaiable",
		STATUS_ENROLL:    "enroll",
		STATUS_CONNECTED: "connected",
		STATUS_ERROR:     "error",
	}
)

type Node struct {
	StorageNode *storage.Node
	State       string // string value of status
	status      int
	logging     bool // logging state is true only when the console session is started
	console     *Console
	ready       chan bool // indicate session has been established with remote
	rwLock      *sync.RWMutex
	reserve     int
}

func NewNode(storNode *storage.Node) *Node {
	node := new(Node)
	node.ready = make(chan bool, 0) // block client
	node.status = STATUS_AVAIABLE
	node.StorageNode = storNode
	node.logging = false
	node.rwLock = new(sync.RWMutex)
	node.reserve = common.TYPE_NO_LOCK
	return node
}

func NewNodeFrom(pbNode *pb.Node) *Node {
	node := new(Node)
	node.StorageNode = new(storage.Node)
	node.StorageNode.Name = pbNode.Name
	node.StorageNode.Driver = pbNode.Driver
	node.StorageNode.Ondemand = pbNode.Ondemand
	node.StorageNode.Params = pbNode.Params
	node.State = STATUS_MAP[int(pbNode.Status)]
	return node
}

func (node *Node) Init() {
	node.rwLock = new(sync.RWMutex)
	node.reserve = common.TYPE_NO_LOCK
	node.ready = make(chan bool, 0) // block client
	node.status = STATUS_AVAIABLE
}

func (node *Node) Validate() error {
	if _, ok := plugins.DRIVER_VALIDATE_MAP[node.StorageNode.Driver]; !ok {
		return errors.New(fmt.Sprintf("Could find driver %s in the supported dictionary", node.StorageNode.Driver))
	}
	if err := plugins.Validate(node.StorageNode.Driver, node.StorageNode.Name, node.StorageNode.Params); err != nil {
		return err
	}
	return nil
}

func (node *Node) restartConsole() {
	if _, ok := nodeManager.Nodes[node.StorageNode.Name]; !ok {
		plog.WarningNode(node.StorageNode.Name, "node has alrealy been removed.")
		return
	}
	if err := node.RequireLock(false); err != nil {
		plog.ErrorNode(node.StorageNode.Name, fmt.Sprintf("Could not start console, error: %s", err.Error()))
		return
	}
	if node.GetStatus() == STATUS_CONNECTED {
		node.Release(false)
		return
	}
	if node.logging == false && node.status == STATUS_AVAIABLE {
		// Do not reconnect if stop logging request received from user
		node.Release(false)
		return
	}
	go node.startConsole()
	if err := common.TimeoutChan(node.ready, serverConfig.Console.TargetTimeout); err != nil {
		plog.ErrorNode(node.StorageNode.Name, fmt.Sprintf("Could not start console, error: %s.", err))
		node.Release(false)
		return
	}
}

func (node *Node) startConsole() {
	var consolePlugin plugins.ConsolePlugin
	var err error
	var baseSession *plugins.BaseSession
	consolePlugin, err = plugins.StartConsole(node.StorageNode.Driver, node.StorageNode.Name, node.StorageNode.Params)
	if err != nil {
		node.status = STATUS_ERROR
		node.ready <- false
		plog.ErrorNode(node.StorageNode.Name, fmt.Sprintf("Could not start console, wait %d seconds and try again, error:%s",
			serverConfig.Console.ReconnectInterval, err.Error()))
		time.Sleep(time.Duration(serverConfig.Console.ReconnectInterval) * time.Second)
		go node.restartConsole()
		return
	}
	baseSession, err = consolePlugin.Start()
	if err != nil {
		node.status = STATUS_ERROR
		node.ready <- false
		plog.ErrorNode(node.StorageNode.Name, fmt.Sprintf("Could not start console, wait %d seconds and try again, error:%s",
			serverConfig.Console.ReconnectInterval, err.Error()))
		time.Sleep(time.Duration(serverConfig.Console.ReconnectInterval) * time.Second)
		go node.restartConsole()
		return
	}
	console := NewConsole(baseSession, node)
	node.console = console
	console.Start()
	if node.StorageNode.Ondemand == false {
		plog.InfoNode(node.StorageNode.Name, "Start console again due to the ondemand setting.")
		time.Sleep(time.Duration(serverConfig.Console.ReconnectInterval) * time.Second)
		node.restartConsole()
	}
}

// has lock
func (node *Node) StartConsole() {
	if err := node.RequireLock(false); err != nil {
		plog.ErrorNode(node.StorageNode.Name, fmt.Sprintf("Could not start console, error: %s", err.Error()))
		return
	}
	if node.GetStatus() == STATUS_CONNECTED {
		node.Release(false)
		return
	}
	go node.startConsole()
	// open a new groutine to make the rest request asynchronous
	go func() {
		if err := common.TimeoutChan(node.GetReadyChan(), serverConfig.Console.TargetTimeout); err != nil {
			plog.ErrorNode(node.StorageNode.Name, err)
		}
		node.Release(false)
	}()
}

func (node *Node) stopConsole() {
	if node.console == nil {
		plog.WarningNode(node.StorageNode.Name, "Console is not started.")
		return
	}
	node.logging = false
	node.console.Stop()
	node.status = STATUS_AVAIABLE
}

// has lock
func (node *Node) StopConsole() {
	if err := node.RequireLock(false); err != nil {
		nodeManager.RWlock.Unlock()
		return
	}
	if node.GetStatus() == STATUS_CONNECTED {
		node.stopConsole()
	}
	node.Release(false)
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

func (node *Node) SetLoggingState(state bool) {
	node.logging = state
}

func (node *Node) RequireLock(share bool) error {
	return common.RequireLock(&node.reserve, node.rwLock, share)
}

func (node *Node) Release(share bool) error {
	return common.ReleaseLock(&node.reserve, node.rwLock, share)
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
	var err error
	plog.Debug("New client connection received.")
	clientTimeout := time.Duration(serverConfig.Console.ClientTimeout)
	size, err := c.ReceiveIntTimeout(conn.(net.Conn), clientTimeout)
	if err != nil {
		conn.(net.Conn).Close()
		return
	}
	b, err := c.ReceiveBytesTimeout(conn.(net.Conn), size, clientTimeout)
	if err != nil {
		conn.(net.Conn).Close()
		return
	}
	data := make(map[string]string)
	if err := json.Unmarshal(b, &data); err != nil {
		plog.Error(err)
		err = c.SendIntWithTimeout(conn.(net.Conn), STATUS_ERROR, clientTimeout)
		if err != nil {
			plog.Error(err)
		}
		conn.(net.Conn).Close()
		return
	}
	if _, ok := nodeManager.Nodes[data["name"]]; !ok {
		defer conn.(net.Conn).Close()
		if !nodeManager.stor.SupportWatcher() {
			plog.ErrorNode(data["name"], "Could not find this node.")
			err = c.SendIntWithTimeout(conn.(net.Conn), STATUS_ERROR, clientTimeout)
			if err != nil {
				plog.ErrorNode(data["name"], err)
			}
			return
		} else {
			nodeWithHost := nodeManager.stor.ListNodeWithHost()
			if _, ok := nodeWithHost[data["name"]]; !ok {
				plog.ErrorNode(data["name"], "Could not find this node.")
				err = c.SendIntWithTimeout(conn.(net.Conn), STATUS_ERROR, clientTimeout)
				if err != nil {
					plog.ErrorNode(data["name"], err)
				}
				return
			}
			host := nodeWithHost[data["name"]]
			err = c.SendIntWithTimeout(conn.(net.Conn), STATUS_REDIRECT, clientTimeout)
			if err != nil {
				plog.ErrorNode(data["name"], err)
				return
			}
			err = c.SendByteWithLength(conn.(net.Conn), []byte(host))
			if err != nil {
				plog.ErrorNode(data["name"], err)
				return
			}
			plog.InfoNode(data["name"], fmt.Sprintf("Redirect the session to %s", host))
		}
		return
	}
	node := nodeManager.Nodes[data["name"]]
	if data["command"] == COMMAND_START_CONSOLE {
		if node.status != STATUS_CONNECTED {
			if err = node.RequireLock(false); err != nil {
				plog.ErrorNode(node.StorageNode.Name, fmt.Sprintf("Could not start console, error: %s.", err))
				err = c.SendIntWithTimeout(conn.(net.Conn), STATUS_ERROR, clientTimeout)
				if err != nil {
					plog.ErrorNode(data["name"], err)
				}
				conn.(net.Conn).Close()
				return
			}
			if node.status == STATUS_CONNECTED {
				node.Release(false)
			} else {
				go node.startConsole()
				if err = common.TimeoutChan(node.ready, serverConfig.Console.TargetTimeout); err != nil {
					plog.ErrorNode(node.StorageNode.Name, fmt.Sprintf("Could not start console, error: %s.", err))
					node.Release(false)
					err = c.SendIntWithTimeout(conn.(net.Conn), STATUS_ERROR, clientTimeout)
					if err != nil {
						plog.ErrorNode(data["name"], err)
					}
					conn.(net.Conn).Close()
					return
				}
			}
		}
		if node.status == STATUS_CONNECTED {
			plog.InfoNode(node.StorageNode.Name, "Register client connection successfully.")
			// reply success message to the client
			err := c.SendIntWithTimeout(conn.(net.Conn), STATUS_CONNECTED, clientTimeout)
			if err != nil {
				plog.ErrorNode(data["name"], err)
			}
			node.console.Accept(conn.(net.Conn))
		} else {
			err := c.SendIntWithTimeout(conn.(net.Conn), STATUS_ERROR, clientTimeout)
			if err != nil {
				plog.ErrorNode(data["name"], err)
			}
			conn.(net.Conn).Close()
		}
	}
}

func (c *ConsoleServer) Listen() {
	var listener net.Listener
	var err error
	if serverConfig.Global.SSLCACertFile != "" && serverConfig.Global.SSLKeyFile != "" && serverConfig.Global.SSLCertFile != "" {
		tlsConfig, err := common.LoadServerTlsConfig(serverConfig.Global.SSLCertFile,
			serverConfig.Global.SSLKeyFile, serverConfig.Global.SSLCACertFile)
		if err != nil {
			panic(err)
		}
		listener, err = tls.Listen("tcp", fmt.Sprintf("%s:%s", c.host, c.port), tlsConfig)
	} else {
		listener, err = net.Listen("tcp", fmt.Sprintf("%s:%s", c.host, c.port))
	}
	if err != nil {
		plog.Error(err)
		return
	}
	c.registerSignal()
	for {
		conn, err := listener.Accept()
		if err != nil {
			plog.Error(err)
			return
		}
		go c.handle(conn)
	}
}

func (c *ConsoleServer) registerSignal() {
	exitHandler := func(s os.Signal, arg interface{}) {
		plog.Info(fmt.Sprintf("Handle signal: %v\n", s))
		os.Exit(1)
	}
	reloadHandler := func(s os.Signal, arg interface{}) {
		plog.Info(fmt.Sprintf("Handle signal: %v, reload configuration file\n", s))
		if common.CONF_FILE != "" {
			common.InitServerConfig(common.CONF_FILE)
			common.SetLogLevel(serverConfig.Global.LogLevel)
		}
	}
	ignoreHandler := func(s os.Signal, arg interface{}) {}
	signalSet := common.GetSignalSet()
	signalSet.Register(syscall.SIGINT, exitHandler)
	signalSet.Register(syscall.SIGTERM, exitHandler)
	signalSet.Register(syscall.SIGHUP, reloadHandler)
	signalSet.Register(syscall.SIGCHLD, ignoreHandler)
	signalSet.Register(syscall.SIGWINCH, ignoreHandler)
	go common.DoSignal()
}

type NodeManager struct {
	Nodes     map[string]*Node
	RWlock    *sync.RWMutex
	stor      storage.StorInterface
	rpcServer *ConsoleRPCServer
}

func GetNodeManager() *NodeManager {
	if nodeManager == nil {
		nodeManager = new(NodeManager)
		nodeManager.Nodes = make(map[string]*Node)
		nodeManager.RWlock = new(sync.RWMutex)
		consoleServer := NewConsoleServer(serverConfig.Global.Host, serverConfig.Console.Port)
		stor, err := storage.NewStorage(serverConfig.Global.StorageType)
		if err != nil {
			panic(err)
		}
		nodeManager.stor = stor
		nodeManager.stor.ImportNodes()
		nodeManager.fromStorNodes()
		runtime.GOMAXPROCS(serverConfig.Global.Worker)
		nodeManager.initNodes()
		go nodeManager.PersistWatcher()
		go consoleServer.Listen()
		if nodeManager.stor.SupportWatcher() {
			nodeManager.rpcServer = newConsoleRPCServer()
			nodeManager.rpcServer.serve()
		}
	}
	return nodeManager
}

func (m *NodeManager) fromStorNodes() {
	m.RWlock.Lock()
	for k, v := range m.stor.GetNodes() {
		var node *Node
		if _, ok := m.Nodes[k]; !ok {
			node = NewNode(v)
		} else {
			node.StorageNode = v
		}
		m.Nodes[k] = node
	}
	m.RWlock.Unlock()
}

func (m *NodeManager) toStorNodes() map[string]*storage.Node {
	storNodes := make(map[string]*storage.Node)
	m.RWlock.RLock()
	for k, v := range m.Nodes {
		storNodes[k] = v.StorageNode
	}
	m.RWlock.RUnlock()
	return storNodes
}

func (m *NodeManager) initNodes() {
	for _, v := range nodeManager.Nodes {
		node := v
		node.Init()
		if node.StorageNode.Ondemand == false {
			go func() {
				if err := node.RequireLock(false); err != nil {
					plog.WarningNode(node.StorageNode.Name, "Conflict while starting console.")
					return
				}
				if node.status == STATUS_CONNECTED {
					plog.WarningNode(node.StorageNode.Name, "Console has already been started.")
					node.Release(false)
					return
				}
				go node.startConsole()
				if err := common.TimeoutChan(node.GetReadyChan(),
					serverConfig.Console.TargetTimeout); err != nil {
					plog.ErrorNode(node.StorageNode.Name, err)
				}
				node.Release(false)
			}()
		}
	}
}

func (m *NodeManager) ListNode() map[string][]map[string]string {
	nodes := make(map[string][]map[string]string)
	nodes["nodes"] = make([]map[string]string, 0)
	if !m.stor.SupportWatcher() {
		for _, node := range nodeManager.Nodes {
			nodeMap := make(map[string]string)
			nodeMap["name"] = node.StorageNode.Name
			nodeMap["host"] = serverConfig.Global.Host
			nodes["nodes"] = append(nodes["nodes"], nodeMap)
		}
	} else {
		nodeWithHost := m.stor.ListNodeWithHost()
		for node, host := range nodeWithHost {
			nodeMap := make(map[string]string)
			nodeMap["name"] = node
			nodeMap["host"] = host
			nodes["nodes"] = append(nodes["nodes"], nodeMap)
		}
	}
	return nodes
}

func (m *NodeManager) ShowNode(name string) (*Node, int, error) {
	var err error
	var node *Node
	if !m.stor.SupportWatcher() {
		if !nodeManager.Exists(name) {
			err = errors.New(fmt.Sprintf("The node %s is not exist.", name))
			return nil, http.StatusBadRequest, err
		}
		node = nodeManager.Nodes[name]
		if err := node.RequireLock(true); err != nil {
			return nil, http.StatusConflict, err
		}
		node.State = STATUS_MAP[node.GetStatus()]
		node.Release(true)
	} else {
		nodeWithHost := m.stor.ListNodeWithHost()
		if nodeWithHost == nil {
			err = errors.New("Could not get host information, please check the storage connection")
			return nil, http.StatusInternalServerError, err
		}
		if _, ok := nodeWithHost[name]; !ok {
			err = errors.New(fmt.Sprintf("Could not get host information for node %s", name))
			return nil, http.StatusBadRequest, err
		}
		cRPCClient := newConsoleRPCClient(nodeWithHost[name], serverConfig.Console.RPCPort)
		pbNode, err := cRPCClient.ShowNode(name)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		node = NewNodeFrom(pbNode)
	}
	return node, http.StatusOK, nil
}

func (m *NodeManager) setConsoleState(nodes []string, state string) map[string]string {
	result := make(map[string]string)
	for _, v := range nodes {
		if v == "" {
			plog.Error("Skip this record as node name is not defined.")
			continue
		}
		m.RWlock.Lock()
		if !m.Exists(v) {
			msg := "Skip this node as it is not exist."
			plog.ErrorNode(v, msg)
			result[v] = msg
			m.RWlock.Unlock()
			continue
		}
		node := m.Nodes[v]
		m.RWlock.Unlock()
		if state == CONSOLE_ON && node.GetStatus() != STATUS_CONNECTED {
			node.StartConsole()
		} else if state == CONSOLE_OFF && node.GetStatus() == STATUS_CONNECTED {
			node.StopConsole()
		}
		result[v] = "Updated"
	}
	return result
}

func (m *NodeManager) SetConsoleState(nodes []string, state string) map[string]string {
	var host, node string
	var nodeList []string
	if !m.stor.SupportWatcher() {
		return m.setConsoleState(nodes, state)
	}
	nodeWithHost := nodeManager.stor.ListNodeWithHost()
	hostNodes := make(map[string][]string, 0)
	for _, node = range nodes {
		if _, ok := nodeWithHost[node]; !ok {
			plog.ErrorNode(node, "Skip this node as it is not exit.")
			continue
		}
		host = nodeWithHost[node]
		//if hostNodes[]
		hostNodes[host] = append(hostNodes[host], node)
	}
	result := make(map[string]string)
	for host, nodeList = range hostNodes {
		cRPCClient := newConsoleRPCClient(host, serverConfig.Console.RPCPort)
		hostResult, err := cRPCClient.SetConsoleState(nodeList, state)
		if err != nil {
			continue
		}
		for k, v := range hostResult {
			result[k] = v
		}
	}
	return result
}

func (m *NodeManager) PostNode(storNode *storage.Node) (int, error) {
	if !m.stor.SupportWatcher() {
		node := NewNode(storNode)
		if err := node.Validate(); err != nil {
			return http.StatusBadRequest, err
		}
		m.RWlock.Lock()
		if m.Exists(node.StorageNode.Name) {
			err := errors.New(fmt.Sprintf("The node name %s is already exist", node.StorageNode.Name))
			m.RWlock.Unlock()
			return http.StatusAlreadyReported, err
		}
		node.SetStatus(STATUS_ENROLL)
		m.Nodes[node.StorageNode.Name] = node
		m.RWlock.Unlock()
		m.NotifyPersist(nil, common.ACTION_NIL)
		plog.InfoNode(node.StorageNode.Name, "Created.")
		if node.StorageNode.Ondemand == false {
			node.StartConsole()
		}
	} else {
		storNodes := make(map[string][]storage.Node)
		storNodes["nodes"] = make([]storage.Node, 0, 1)
		storNodes["nodes"] = append(storNodes["nodes"], *storNode)
		m.NotifyPersist(storNodes, common.ACTION_PUT)
	}
	return http.StatusAccepted, nil
}

func (m *NodeManager) postNodes(storNodes []storage.Node, result map[string]string) {
	for _, v := range storNodes {
		// the silce pointer will be changed with the for loop, create a new variable.
		if v.Name == "" {
			plog.Error("Skip this record as node name is not defined.")
			continue
		}
		if v.Driver == "" {
			msg := "Driver is not defined."
			plog.ErrorNode(v.Name, msg)
			if result != nil {
				result[v.Name] = msg
			}
			continue
		}
		temp := v // the slice in go is a little tricky, use temporary variable to store the value
		node := NewNode(&temp)
		if err := node.Validate(); err != nil {
			msg := "Failed to validate the node property."
			plog.ErrorNode(node.StorageNode.Name, msg)
			if result != nil {
				result[node.StorageNode.Name] = msg
			}

			continue
		}
		m.RWlock.Lock()
		if m.Exists(node.StorageNode.Name) {
			msg := "Skip this node as node is exist."
			plog.ErrorNode(node.StorageNode.Name, msg)
			if result != nil {
				result[node.StorageNode.Name] = msg
			}
			m.RWlock.Unlock()
			continue
		}
		node.SetStatus(STATUS_ENROLL)
		m.Nodes[node.StorageNode.Name] = node
		m.RWlock.Unlock()
		if result != nil {
			result[node.StorageNode.Name] = "Created"
		}
		if node.StorageNode.Ondemand == false {
			node.StartConsole()
		}
	}
}

func (m *NodeManager) PostNodes(storNodes map[string][]storage.Node) map[string]string {
	result := make(map[string]string)
	if !m.stor.SupportWatcher() {
		m.postNodes(storNodes["nodes"], result)
		m.NotifyPersist(nil, common.ACTION_NIL)
	} else {
		for _, v := range storNodes["nodes"] {
			result[v.Name] = "Accept"
		}
		m.NotifyPersist(storNodes, common.ACTION_PUT)
	}
	return result
}

func (m *NodeManager) DeleteNode(nodeName string) (int, error) {
	if !m.stor.SupportWatcher() {
		nodeManager.RWlock.Lock()
		if !nodeManager.Exists(nodeName) {
			nodeManager.RWlock.Unlock()
			return http.StatusBadRequest, errors.New(fmt.Sprintf("Node %s is not exist", nodeName))
		}
		node := nodeManager.Nodes[nodeName]
		if node.GetStatus() == STATUS_CONNECTED {
			go node.StopConsole()
		}
		delete(nodeManager.Nodes, nodeName)
		nodeManager.RWlock.Unlock()
		nodeManager.NotifyPersist(nil, common.ACTION_NIL)
	} else {
		names := make([]string, 0, 1)
		names = append(names, nodeName)
		m.NotifyPersist(names, common.ACTION_DELETE)
	}
	return http.StatusAccepted, nil
}

func (m *NodeManager) deleteNodes(names []string, result map[string]string) {
	for _, v := range names {
		if v == "" {
			plog.Error("Skip this record as node name is not defined.")
			continue
		}
		name := v
		nodeManager.RWlock.Lock()
		if !nodeManager.Exists(name) {
			msg := "Skip this node as node is not exist."
			plog.ErrorNode(name, msg)
			if result != nil {
				result[name] = msg
			}
			nodeManager.RWlock.Unlock()
			continue
		}
		node := nodeManager.Nodes[name]
		if node.GetStatus() == STATUS_CONNECTED {
			node.StopConsole()
		}
		delete(nodeManager.Nodes, name)
		nodeManager.RWlock.Unlock()
		if result != nil {
			result[name] = "Deleted"
		}
	}
}

func (m *NodeManager) DeleteNodes(names []string) map[string]string {
	result := make(map[string]string)
	if !m.stor.SupportWatcher() {
		m.deleteNodes(names, result)
		m.NotifyPersist(nil, common.ACTION_NIL)
	} else {
		for _, name := range names {
			result[name] = "Accept"
		}
		m.NotifyPersist(names, common.ACTION_DELETE)
	}
	return result
}

func (m *NodeManager) NotifyPersist(node interface{}, action int) {
	if !m.stor.SupportWatcher() {
		storNodes := m.toStorNodes()
		m.stor.NotifyPersist(storNodes, common.ACTION_NIL)
		return
	}
	m.stor.NotifyPersist(node, action)
}

func (m *NodeManager) Exists(nodeName string) bool {
	if _, ok := m.Nodes[nodeName]; ok {
		return true
	}
	return false
}

func (m *NodeManager) PersistWatcher() {
	if !m.stor.SupportWatcher() {
		m.stor.PersistWatcher(nil)
		return
	}
	eventChan := make(chan map[int][]byte, 1024)
	go m.stor.PersistWatcher(eventChan)
	for {
		select {
		case eventMap := <-eventChan:
			if _, ok := eventMap[common.ACTION_PUT]; ok {
				storNode := storage.NewNode()
				if err := json.Unmarshal(eventMap[common.ACTION_PUT], storNode); err != nil {
					plog.Error(err)
					return
				}
				storNodes := make([]storage.Node, 0, 1)
				storNodes = append(storNodes, *storNode)
				m.postNodes(storNodes, nil)
			} else if _, ok := eventMap[common.ACTION_DELETE]; ok {
				name := string(eventMap[common.ACTION_DELETE])
				names := make([]string, 0, 1)
				names = append(names, name)
				m.deleteNodes(names, nil)
			} else {
				plog.Error("Internal error")
			}
		}
	}
}
