package console

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/chenglch/goconserver/common"
	pb "github.com/chenglch/goconserver/console/consolepb"
	pl "github.com/chenglch/goconserver/console/pipeline"
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
	STATUS_NOTFOUND
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
		STATUS_NOTFOUND:  "notfound",
		STATUS_ERROR:     "error",
	}
)

type ReadyBuffer struct {
	node string
	last *pl.RemainBuffer
}

type Node struct {
	StorageNode *storage.Node
	State       string // string value of status
	status      int
	logging     bool // indicate whether to reconnect
	console     *Console
	ready       chan bool // indicate session has been established with remote
	rwLock      *sync.RWMutex
	reconnect   chan struct{} //  wait on this channel to get the start action.
	reserve     int
}

func NewNode(storNode *storage.Node) *Node {
	node := new(Node)
	node.ready = make(chan bool, 0) // block client
	node.reconnect = make(chan struct{}, 0)
	node.status = STATUS_AVAIABLE
	node.StorageNode = storNode
	if node.StorageNode.Ondemand == false {
		node.logging = true
	} else {
		node.logging = false
	}
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
	if node.StorageNode.Ondemand == false {
		node.logging = true
	} else {
		node.logging = false
	}
	node.State = STATUS_MAP[int(pbNode.Status)]
	return node
}

func (node *Node) Init() {
	node.rwLock = new(sync.RWMutex)
	node.reserve = common.TYPE_NO_LOCK
	node.ready = make(chan bool, 0) // block client
	node.reconnect = make(chan struct{}, 0)
	node.status = STATUS_AVAIABLE
}

func (node *Node) Validate() error {
	if _, ok := plugins.DRIVER_VALIDATE_MAP[node.StorageNode.Driver]; !ok {
		plog.ErrorNode(node.StorageNode.Name, fmt.Sprintf("Coud not support driver %s", node.StorageNode.Driver))
		return common.ErrDriverNotExist
	}
	if err := plugins.Validate(node.StorageNode.Driver, node.StorageNode.Name, node.StorageNode.Params); err != nil {
		return err
	}
	return nil
}

func (node *Node) restartConsole() {
	var ok bool
	var err error
	for {
		select {
		case _, ok = <-node.reconnect:
			if !ok {
				plog.DebugNode(node.StorageNode.Name, "Exit reconnect goroutine")
				return
			}
			if err = node.RequireLock(false); err != nil {
				plog.ErrorNode(node.StorageNode.Name, err.Error())
				break
			}
			// with lock then check
			if node.GetStatus() == STATUS_CONNECTED {
				node.Release(false)
				break
			}
			if node.logging == false {
				node.Release(false)
				break
			}
			plog.DebugNode(node.StorageNode.Name, "Restart console session.")
			go node.startConsole()
			common.TimeoutChan(node.ready, serverConfig.Console.TargetTimeout)
			node.Release(false)
		}
	}
}

// A little different from StartConsole, this function do not handle the node lock, node ready
// is used to wake up the waiting channel outside.
func (node *Node) startConsole() {
	var consolePlugin plugins.ConsolePlugin
	var err error
	var baseSession *plugins.BaseSession
	if node.console != nil {
		plog.WarningNode(node.StorageNode.Name, "Console has already been started")
		node.ready <- true
		return
	}
	consolePlugin, err = plugins.StartConsole(node.StorageNode.Driver, node.StorageNode.Name, node.StorageNode.Params)
	if err != nil {
		node.status = STATUS_ERROR
		node.ready <- false
		if node.StorageNode.Ondemand == false {
			go func() {
				plog.DebugNode(node.StorageNode.Name, fmt.Sprintf("Could not start console, wait %d seconds and try again, error:%s",
					serverConfig.Console.ReconnectInterval, err.Error()))
				time.Sleep(time.Duration(serverConfig.Console.ReconnectInterval) * time.Second)
				common.SafeSend(node.reconnect, struct{}{})
			}()
		}
		return
	}
	baseSession, err = consolePlugin.Start()
	if err != nil {
		node.status = STATUS_ERROR
		node.ready <- false
		if node.StorageNode.Ondemand == false {
			go func() {
				plog.DebugNode(node.StorageNode.Name, fmt.Sprintf("Could not start console, wait %d seconds and try again, error:%s",
					serverConfig.Console.ReconnectInterval, err.Error()))
				time.Sleep(time.Duration(serverConfig.Console.ReconnectInterval) * time.Second)
				common.SafeSend(node.reconnect, struct{}{})
			}()
		}
		return
	}
	node.console = NewConsole(baseSession, node)
	// console.Start will block until the console session is closed.
	node.console.Start()
	node.console = nil
	// only when the Ondemand is false, console session will be restarted automatically
	if node.StorageNode.Ondemand == false {
		go func() {
			plog.InfoNode(node.StorageNode.Name, "Start console again due to the ondemand setting.")
			time.Sleep(time.Duration(serverConfig.Console.ReconnectInterval) * time.Second)
			common.SafeSend(node.reconnect, struct{}{})
		}()
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
	node.console.Stop()
	node.status = STATUS_AVAIABLE
	node.console = nil
}

// has lock
func (node *Node) StopConsole() error {
	if err := node.RequireLock(false); err != nil {
		plog.ErrorNode(node.StorageNode.Name, fmt.Sprintf("Unable to stop console session, error:%v", err))
		return err
	}
	if node.GetStatus() == STATUS_CONNECTED {
		node.stopConsole()
	}
	node.Release(false)
	return nil
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
	host, port string
}

func NewConsoleServer(host string, port string) *ConsoleServer {
	nodeManager = GetNodeManager()
	return &ConsoleServer{host: host, port: port}
}

func (c *ConsoleServer) getConnectionInfo(conn interface{}) (string, string) {
	var node, command string
	var ok bool
	clientTimeout := time.Duration(serverConfig.Console.ClientTimeout)
	size, err := common.Network.ReceiveIntTimeout(conn.(net.Conn), clientTimeout)
	if err != nil {
		conn.(net.Conn).Close()
		return "", ""
	}
	b, err := common.Network.ReceiveBytesTimeout(conn.(net.Conn), size, clientTimeout)
	if err != nil {
		return "", ""
	}
	plog.Debug(fmt.Sprintf("Receive connection from client: %s", string(b)))
	data := make(map[string]string)
	if err := json.Unmarshal(b, &data); err != nil {
		plog.Error(err)
		err = common.Network.SendIntWithTimeout(conn.(net.Conn), STATUS_ERROR, clientTimeout)
		if err != nil {
			plog.Error(err)
		}
		return "", ""
	}
	if node, ok = data["name"]; !ok {
		plog.Error("Could not get the node from client")
		return "", ""
	}
	if command, ok = data["command"]; !ok {
		plog.Error("Could not get the command from client")
		return "", ""
	}
	return node, command
}

func (c *ConsoleServer) redirect(conn interface{}, node string) {
	var err error
	nodeWithHost := nodeManager.stor.ListNodeWithHost()
	clientTimeout := time.Duration(serverConfig.Console.ClientTimeout)
	if _, ok := nodeWithHost[node]; !ok {
		plog.ErrorNode(node, "Could not find this node.")
		err = common.Network.SendIntWithTimeout(conn.(net.Conn), STATUS_NOTFOUND, clientTimeout)
		if err != nil {
			plog.ErrorNode(node, err)
		}
		return
	}
	host := nodeWithHost[node]
	err = common.Network.SendIntWithTimeout(conn.(net.Conn), STATUS_REDIRECT, clientTimeout)
	if err != nil {
		plog.ErrorNode(node, err)
		return
	}
	err = common.Network.SendByteWithLength(conn.(net.Conn), []byte(host))
	if err != nil {
		plog.ErrorNode(node, err)
		return
	}
	plog.InfoNode(node, fmt.Sprintf("Redirect the session to %s", host))
}

func (c *ConsoleServer) handle(conn interface{}) {
	var err error
	clientTimeout := time.Duration(serverConfig.Console.ClientTimeout)
	plog.Debug("New client connection received.")
	name, command := c.getConnectionInfo(conn)
	if name == "" && command == "" {
		conn.(net.Conn).Close()
		return
	}
	nodeManager.RWlock.RLock()
	if _, ok := nodeManager.Nodes[name]; !ok {
		defer conn.(net.Conn).Close()
		if !nodeManager.stor.SupportWatcher() {
			plog.ErrorNode(name, "Could not find this node.")
			err = common.Network.SendIntWithTimeout(conn.(net.Conn), STATUS_NOTFOUND, clientTimeout)
			if err != nil {
				plog.ErrorNode(name, err)
			}
			nodeManager.RWlock.RUnlock()
			return
		} else {
			c.redirect(conn, name)
		}
		nodeManager.RWlock.RUnlock()
		return
	}
	node := nodeManager.Nodes[name]
	nodeManager.RWlock.RUnlock()
	if command == COMMAND_START_CONSOLE {
		if node.status != STATUS_CONNECTED {
			if err = node.RequireLock(false); err != nil {
				plog.ErrorNode(node.StorageNode.Name, fmt.Sprintf("Could not start console, error: %s.", err))
				err = common.Network.SendIntWithTimeout(conn.(net.Conn), STATUS_ERROR, clientTimeout)
				if err != nil {
					plog.ErrorNode(name, err)
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
					err = common.Network.SendIntWithTimeout(conn.(net.Conn), STATUS_ERROR, clientTimeout)
					if err != nil {
						plog.ErrorNode(name, err)
					}
					conn.(net.Conn).Close()
					return
				}
				node.Release(false)
			}
		}
		if node.status == STATUS_CONNECTED {
			plog.InfoNode(node.StorageNode.Name, "Register client connection successfully.")
			// reply success message to the client
			err := common.Network.SendIntWithTimeout(conn.(net.Conn), STATUS_CONNECTED, clientTimeout)
			if err != nil {
				plog.ErrorNode(name, err)
				conn.(net.Conn).Close()
				return
			}
			node.console.Accept(conn.(net.Conn))
		} else {
			err := common.Network.SendIntWithTimeout(conn.(net.Conn), STATUS_ERROR, clientTimeout)
			if err != nil {
				plog.ErrorNode(name, err)
				conn.(net.Conn).Close()
				return
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
		common.CloseLogger()
		os.Exit(1)
	}
	reloadHandler := func(s os.Signal, arg interface{}) {
		plog.Info(fmt.Sprintf("Handle signal: %v, reload configuration file\n", s))
		if common.CONF_FILE != "" {
			common.InitServerConfig(common.CONF_FILE)
			common.CloseLogger()
			common.InitLogger()
		}
	}
	ignoreHandler := func(s os.Signal, arg interface{}) {}
	signalSet := common.GetSignalSet()
	signalSet.Register(syscall.SIGINT, exitHandler)
	signalSet.Register(syscall.SIGTERM, exitHandler)
	signalSet.Register(syscall.SIGHUP, reloadHandler)
	signalSet.Register(syscall.SIGPIPE, ignoreHandler)
	go common.DoSignal(nil)
}

type NodeManager struct {
	Nodes     map[string]*Node
	RWlock    *sync.RWMutex
	stor      storage.StorInterface
	rpcServer *ConsoleRPCServer
	pipeline  *pl.Pipeline
	hostname  string
}

func GetNodeManager() *NodeManager {
	if nodeManager == nil {
		nodeManager = new(NodeManager)
		hostname, err := os.Hostname()
		if err != nil {
			panic(err)
		}
		nodeManager.hostname = hostname
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
		// start loggers
		nodeManager.pipeline, err = pl.NewPipeline(&serverConfig.Console.Loggers)
		if err != nil {
			panic(err)
		}
		// for linelogger to send the last buffer
		go nodeManager.PeriodicTask()
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
		if node.StorageNode.Ondemand == false {
			node.logging = true
		} else {
			node.logging = false
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
	nodeManager.RWlock.RLock()
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
				go node.restartConsole()
				go node.startConsole()
				if err := common.TimeoutChan(node.GetReadyChan(),
					serverConfig.Console.TargetTimeout); err != nil {
					plog.ErrorNode(node.StorageNode.Name, err)
				}
				node.Release(false)
			}()
		}
	}
	nodeManager.RWlock.RUnlock()
}

func (m *NodeManager) ListNode() map[string][]map[string]string {
	nodes := make(map[string][]map[string]string)
	nodes["nodes"] = make([]map[string]string, 0)
	if !m.stor.SupportWatcher() {
		m.RWlock.RLock()
		for _, node := range nodeManager.Nodes {
			nodeMap := make(map[string]string)
			nodeMap["name"] = node.StorageNode.Name
			nodeMap["host"] = m.hostname
			nodeMap["state"] = STATUS_MAP[node.GetStatus()]
			nodes["nodes"] = append(nodes["nodes"], nodeMap)
		}
		m.RWlock.RUnlock()
	} else {
		var host string
		var node string
		nodeWithHost := m.stor.ListNodeWithHost()
		hosts := m.stor.GetHosts()
		if hosts == nil {
			return nodes
		}
		statusMap := make(map[string]int)
		for _, host = range hosts {
			rpcClient := newConsoleRPCClient(host, serverConfig.Console.RPCPort)
			rpcResult, err := rpcClient.ListNodesStatus()
			if err != nil {
				plog.Error(fmt.Sprintf("Could not get rpc result from host %s, Error: %s", host, err.Error()))
				continue
			}
			for k, v := range rpcResult {
				statusMap[k] = v
			}
		}
		for node, host = range nodeWithHost {
			nodeMap := make(map[string]string)
			nodeMap["name"] = node
			nodeMap["host"] = host
			if status, ok := statusMap[node]; ok {
				nodeMap["state"] = STATUS_MAP[status]
			}
			nodes["nodes"] = append(nodes["nodes"], nodeMap)
		}
	}
	return nodes
}

func (m *NodeManager) ShowNode(name string) (*Node, int, string) {
	var node *Node
	if !m.stor.SupportWatcher() {
		if !nodeManager.Exists(name) {
			return nil, http.StatusBadRequest, fmt.Sprintf("The node %s is not exist.", name)
		}
		node = nodeManager.Nodes[name]
		node.State = STATUS_MAP[node.GetStatus()]
	} else {
		nodeWithHost := m.stor.ListNodeWithHost()
		if nodeWithHost == nil {
			return nil, http.StatusInternalServerError, "Could not get host information, please check the storage connection"
		}
		if _, ok := nodeWithHost[name]; !ok {
			return nil, http.StatusBadRequest, fmt.Sprintf("Could not get host information for node %s", name)
		}
		cRPCClient := newConsoleRPCClient(nodeWithHost[name], serverConfig.Console.RPCPort)
		pbNode, err := cRPCClient.ShowNode(name)
		if err != nil {
			return nil, http.StatusInternalServerError, err.Error()
		}
		node = NewNodeFrom(pbNode)
	}
	return node, http.StatusOK, ""
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
		if state == CONSOLE_ON {
			node.logging = true
			if node.GetStatus() != STATUS_CONNECTED {
				node.StartConsole()
			}
		} else if state == CONSOLE_OFF {
			node.logging = false
			if node.GetStatus() != STATUS_CONNECTED {
				continue
			}
			if err := node.StopConsole(); err != nil {
				continue
			}
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
			plog.ErrorNode(node, "Skip this node as it is not exist.")
			continue
		}
		host = nodeWithHost[node]
		//if hostNodes[]
		hostNodes[host] = append(hostNodes[host], node)
	}
	result := make(map[string]string)
	for host, nodeList = range hostNodes {
		rpcClient := newConsoleRPCClient(host, serverConfig.Console.RPCPort)
		hostResult, err := rpcClient.SetConsoleState(nodeList, state)
		if err != nil {
			continue
		}
		for k, v := range hostResult {
			result[k] = v
		}
	}
	return result
}

func (m *NodeManager) PostNode(storNode *storage.Node) (int, string) {
	if !m.stor.SupportWatcher() {
		node := NewNode(storNode)
		if err := node.Validate(); err != nil {
			return http.StatusBadRequest, err.Error()
		}
		m.RWlock.Lock()
		if m.Exists(node.StorageNode.Name) {
			m.RWlock.Unlock()
			return http.StatusConflict, fmt.Sprintf("The node name %s is already exist", node.StorageNode.Name)
		}
		node.SetStatus(STATUS_ENROLL)
		m.Nodes[node.StorageNode.Name] = node
		m.RWlock.Unlock()
		m.NotifyPersist(nil, common.ACTION_NIL)
		plog.InfoNode(node.StorageNode.Name, "Created.")
		if node.StorageNode.Ondemand == false {
			go node.restartConsole()
			node.StartConsole()

		}
	} else {
		storNodes := make(map[string][]storage.Node)
		storNodes["nodes"] = make([]storage.Node, 0, 1)
		storNodes["nodes"] = append(storNodes["nodes"], *storNode)
		m.NotifyPersist(storNodes, common.ACTION_PUT)
	}
	return http.StatusAccepted, ""
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
			plog.ErrorNode(node.StorageNode.Name, err.Error())
			if result != nil {
				result[node.StorageNode.Name] = err.Error()
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
			go node.restartConsole()
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

func (m *NodeManager) DeleteNode(nodeName string) (int, string) {
	if !m.stor.SupportWatcher() {
		nodeManager.RWlock.Lock()
		node, ok := nodeManager.Nodes[nodeName]
		if !ok {
			nodeManager.RWlock.Unlock()
			return http.StatusBadRequest, fmt.Sprintf("Node %s is not exist", nodeName)
		}
		if node.StorageNode.Ondemand == false {
			close(node.reconnect)
		}
		if node.GetStatus() == STATUS_CONNECTED {
			if err := node.StopConsole(); err != nil {
				nodeManager.RWlock.Unlock()
				return http.StatusConflict, err.Error()
			}
		}
		delete(nodeManager.Nodes, nodeName)
		nodeManager.RWlock.Unlock()
		nodeManager.NotifyPersist(nil, common.ACTION_NIL)
	} else {
		names := make([]string, 0, 1)
		names = append(names, nodeName)
		m.NotifyPersist(names, common.ACTION_DELETE)
	}
	return http.StatusAccepted, ""
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
		if node.StorageNode.Ondemand == false {
			close(node.reconnect)
		}
		if node.GetStatus() == STATUS_CONNECTED {
			if err := node.StopConsole(); err != nil {
				nodeManager.RWlock.Unlock()
				continue
			}
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

func (m *NodeManager) Replay(name string) (string, int, string) {
	var content string
	var err error
	if !m.stor.SupportWatcher() {
		if !m.Exists(name) {
			return "", http.StatusBadRequest, fmt.Sprintf("The node %s is not exist.", name)
		}
		// TODO: make replaylines more flexible
		content, err = nodeManager.pipeline.Fetch(name, serverConfig.Console.ReplayLines)
		if err != nil {
			return "", http.StatusInternalServerError, err.Error()
		}
	} else {
		nodeWithHost := m.stor.ListNodeWithHost()
		if nodeWithHost == nil {
			return "", http.StatusInternalServerError, "Could not get host information, please check the storage connection"
		}
		if _, ok := nodeWithHost[name]; !ok {
			return "", http.StatusBadRequest, fmt.Sprintf("Could not get host information for node %s", name)
		}
		rpcClient := newConsoleRPCClient(nodeWithHost[name], serverConfig.Console.RPCPort)
		content, err = rpcClient.GetReplayContent(name)
		if err != nil {
			return "", http.StatusInternalServerError, err.Error()
		}
	}
	return content, http.StatusOK, ""
}

func (m *NodeManager) ListUser(name string) (map[string][]string, int, string) {
	var node *Node
	var users []string
	var err error
	var ok bool
	ret := make(map[string][]string)
	if !m.stor.SupportWatcher() {
		m.RWlock.RLock()
		if node, ok = m.Nodes[name]; !ok {
			m.RWlock.RUnlock()
			return nil, http.StatusBadRequest, fmt.Sprintf("The node %s is not exist.", name)
		}
		m.RWlock.RUnlock()
		users = node.console.ListSessionUser()
	} else {
		nodeWithHost := m.stor.ListNodeWithHost()
		if nodeWithHost == nil {
			return nil, http.StatusInternalServerError, "Could not get host information, please check the storage connection"
		}
		if _, ok := nodeWithHost[name]; !ok {
			return nil, http.StatusBadRequest, fmt.Sprintf("Could not get host information for node %s", name)
		}
		rpcClient := newConsoleRPCClient(nodeWithHost[name], serverConfig.Console.RPCPort)
		users, err = rpcClient.ListSessionUser(name)
		if err != nil {
			return nil, http.StatusInternalServerError, err.Error()
		}
	}
	ret["users"] = users
	return ret, http.StatusOK, ""
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

// for linelogger to send the last buffer
func (m *NodeManager) PeriodicTask() {
	if m.pipeline.Periodic == false {
		return
	}
	plog.Info("Starting peridic task")
	tick := time.Tick(common.PERIODIC_INTERVAL)
	for {
		<-tick
		current := time.Now()
		plog.Debug("Periodic task is running")
		readyList := make([]*ReadyBuffer, 0)
		m.RWlock.RLock()
		for k, v := range m.Nodes {
			console := v.console
			if console == nil {
				continue
			}
			if console.last.Buf != nil && console.last.Deadline.After(current) {
				readyBuf := &ReadyBuffer{node: k, last: console.last}
				readyList = append(readyList, readyBuf)
			}
		}
		m.RWlock.RUnlock()
		for _, readyBuf := range readyList {
			m.pipeline.PromptLast(readyBuf.node, readyBuf.last)
		}
	}
}
