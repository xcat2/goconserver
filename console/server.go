package console

import (
	"crypto/tls"
	"fmt"
	"github.com/xcat2/goconserver/common"
	pb "github.com/xcat2/goconserver/console/consolepb"
	pl "github.com/xcat2/goconserver/console/pipeline"
	"github.com/xcat2/goconserver/plugins"
	"github.com/xcat2/goconserver/storage"
	"golang.org/x/net/websocket"
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

	CONSOLE_ON  = "on"
	CONSOLE_OFF = "off"
)

var (
	plog          = common.GetLogger("github.com/xcat2/goconserver/console")
	nodeManager   *NodeManager
	consoleServer *ConsoleServer
	serverConfig  = common.GetServerConfig()
	STATUS_MAP    = map[int]string{
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

func NewNodeFromStor(storNode *storage.Node) *Node {
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

func NewNodeFromProto(pbNode *pb.Node) *Node {
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

// the storage information has been initialized, initialize the other fields in memory.
func (self *Node) init() {
	self.rwLock = new(sync.RWMutex)
	self.reserve = common.TYPE_NO_LOCK
	self.ready = make(chan bool, 0) // block client
	self.reconnect = make(chan struct{}, 0)
	self.status = STATUS_AVAIABLE
}

func (self *Node) Validate() error {
	if _, ok := plugins.DRIVER_VALIDATE_MAP[self.StorageNode.Driver]; !ok {
		plog.ErrorNode(self.StorageNode.Name, fmt.Sprintf("Coud not support driver %s", self.StorageNode.Driver))
		return common.ErrDriverNotExist
	}
	if err := plugins.Validate(self.StorageNode.Driver, self.StorageNode.Name, self.StorageNode.Params); err != nil {
		return err
	}
	return nil
}

// if ondemand is false, start this goroutine to help restart the console
func (self *Node) restartMonitor() {
	var ok bool
	var err error
	for {
		select {
		case _, ok = <-self.reconnect:
			if !ok {
				plog.DebugNode(self.StorageNode.Name, "Exit reconnect goroutine")
				return
			}
			// before start console, both request from client and reconnecting monitor try to get the lock at first
			// so that only one startConsole goroutine is running for the node.
			if err = self.RequireLock(false); err != nil {
				plog.ErrorNode(self.StorageNode.Name, err.Error())
				break
			}
			// with lock then check
			if self.GetStatus() == STATUS_CONNECTED {
				self.Release(false)
				break
			}
			if self.logging == false {
				self.Release(false)
				break
			}
			plog.DebugNode(self.StorageNode.Name, "Restart console session.")
			go self.startConsole()
			common.TimeoutChan(self.ready, serverConfig.Console.TargetTimeout)
			self.Release(false)
		}
	}
}

// A little different from StartConsole, this function do not handle the node lock, node ready
// is used to wake up the waiting channel outside.
func (self *Node) startConsole() {
	var consolePlugin plugins.ConsolePlugin
	var err error
	var baseSession *plugins.BaseSession
	if self.console != nil {
		plog.WarningNode(self.StorageNode.Name, "Console has already been started")
		self.ready <- true
		return
	}
	consolePlugin, err = plugins.StartConsole(self.StorageNode.Driver, self.StorageNode.Name, self.StorageNode.Params)
	if err != nil {
		self.status = STATUS_ERROR
		self.ready <- false
		if self.StorageNode.Ondemand == false {
			go func() {
				plog.DebugNode(self.StorageNode.Name, fmt.Sprintf("Could not start console, wait %d seconds and try again, error:%s",
					serverConfig.Console.ReconnectInterval, err.Error()))
				time.Sleep(time.Duration(serverConfig.Console.ReconnectInterval) * time.Second)
				common.SafeSend(self.reconnect, struct{}{})
			}()
		}
		return
	}
	baseSession, err = consolePlugin.Start()
	if err != nil {
		self.status = STATUS_ERROR
		self.ready <- false
		if self.StorageNode.Ondemand == false {
			go func() {
				plog.DebugNode(self.StorageNode.Name, fmt.Sprintf("Could not start console, wait %d seconds and try again, error:%s",
					serverConfig.Console.ReconnectInterval, err.Error()))
				time.Sleep(time.Duration(serverConfig.Console.ReconnectInterval) * time.Second)
				common.SafeSend(self.reconnect, struct{}{})
			}()
		}
		return
	}
	self.console = NewConsole(baseSession, self)
	// console.Start will block until the console session is closed.
	self.console.Start()
	self.console = nil
	// only when the Ondemand is false, console session will be restarted automatically
	if self.StorageNode.Ondemand == false {
		go func() {
			plog.InfoNode(self.StorageNode.Name, "Start console again due to the ondemand setting.")
			time.Sleep(time.Duration(serverConfig.Console.ReconnectInterval) * time.Second)
			common.SafeSend(self.reconnect, struct{}{})
		}()
	}
}

// has lock
func (self *Node) StartConsole() {
	if err := self.RequireLock(false); err != nil {
		plog.ErrorNode(self.StorageNode.Name, fmt.Sprintf("Could not start console, error: %s", err.Error()))
		return
	}
	if self.GetStatus() == STATUS_CONNECTED {
		self.Release(false)
		return
	}
	go self.startConsole()
	// open a new groutine to make the rest request asynchronous
	go func() {
		if err := common.TimeoutChan(self.GetReadyChan(), serverConfig.Console.TargetTimeout); err != nil {
			plog.ErrorNode(self.StorageNode.Name, err)
		}
		self.Release(false)
	}()
}

func (self *Node) stopConsole() {
	if self.console == nil {
		plog.WarningNode(self.StorageNode.Name, "Console is not started.")
		return
	}
	self.console.Stop()
	self.status = STATUS_AVAIABLE
	self.console = nil
}

// has lock
func (self *Node) StopConsole() error {
	if err := self.RequireLock(false); err != nil {
		plog.ErrorNode(self.StorageNode.Name, fmt.Sprintf("Unable to stop console session, error:%v", err))
		return err
	}
	if self.GetStatus() == STATUS_CONNECTED {
		self.stopConsole()
	}
	self.Release(false)
	return nil
}

func (self *Node) SetStatus(status int) {
	self.status = status
}

func (self *Node) GetStatus() int {
	return self.status
}

func (self *Node) GetReadyChan() chan bool {
	return self.ready
}

func (self *Node) SetLoggingState(state bool) {
	self.logging = state
}

func (self *Node) RequireLock(share bool) error {
	return common.RequireLock(&self.reserve, self.rwLock, share)
}

func (self *Node) Release(share bool) error {
	return common.ReleaseLock(&self.reserve, self.rwLock, share)
}

type ConsoleServer struct {
	host, port string
}

func NewConsoleServer(host string, port string) *ConsoleServer {
	nodeManager = GetNodeManager()
	return &ConsoleServer{host: host, port: port}
}

func AcceptWesocketClient(ws *websocket.Conn) error {
	if consoleServer == nil {
		return common.ErrNullObject
	}
	consoleServer.handle(ws)
	return nil
}

func (self *ConsoleServer) handle(conn interface{}) {
	clientTimeout := time.Duration(serverConfig.Console.ClientTimeout)
	plog.Debug("New client connection received.")
	node, err := serverHandshake(conn.(net.Conn))
	if err != nil {
		plog.Error(err)
		return
	}
	if node == nil {
		// may be redirected to the other host
		return
	}
	if node.status != STATUS_CONNECTED {
		// NOTE(chenglch): Get the lock at first, then allow to connect to the console target.
		if err = node.RequireLock(false); err != nil {
			msg := fmt.Sprintf("Could not start console, error: %s.", err)
			plog.ErrorNode(node.StorageNode.Name, msg)
			err = sendProtoMessage(conn.(net.Conn), msg, ACTION_SESSION_ERROR, clientTimeout)
			if err != nil {
				plog.ErrorNode(node.StorageNode.Name, err)
			}
			conn.(net.Conn).Close()
			return
		}
		if node.status == STATUS_CONNECTED {
			node.Release(false)
		} else {
			// NOTE(chenglch): Already got the lock, but the console connection is not established, start
			// console at the backend.
			go node.startConsole()
			if err = common.TimeoutChan(node.ready, serverConfig.Console.TargetTimeout); err != nil {
				node.Release(false)
				msg := fmt.Sprintf("Could not start console, error: %s.", err)
				plog.ErrorNode(node.StorageNode.Name, msg)
				err = sendProtoMessage(conn.(net.Conn), msg, ACTION_SESSION_ERROR, clientTimeout)
				if err != nil {
					plog.ErrorNode(node.StorageNode.Name, err)
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
		err = sendProtoMessage(conn.(net.Conn), "", ACTION_SESSION_OK, clientTimeout)
		if err != nil {
			plog.ErrorNode(node.StorageNode.Name, err)
		}
		node.console.Accept(conn.(net.Conn))
	} else {
		err = sendProtoMessage(conn.(net.Conn), "Could not connect to console", ACTION_SESSION_ERROR, clientTimeout)
		if err != nil {
			plog.ErrorNode(node.StorageNode.Name, err)
		}
		conn.(net.Conn).Close()
	}
}

func (self *ConsoleServer) Listen() {
	var listener net.Listener
	var err error
	if serverConfig.Global.SSLCACertFile != "" && serverConfig.Global.SSLKeyFile != "" && serverConfig.Global.SSLCertFile != "" {
		tlsConfig, err := common.LoadServerTlsConfig(serverConfig.Global.SSLCertFile,
			serverConfig.Global.SSLKeyFile, serverConfig.Global.SSLCACertFile)
		if err != nil {
			panic(err)
		}
		listener, err = tls.Listen("tcp", fmt.Sprintf("%s:%s", self.host, self.port), tlsConfig)
	} else {
		listener, err = net.Listen("tcp", fmt.Sprintf("%s:%s", self.host, self.port))
	}
	if err != nil {
		plog.Error(err)
		return
	}
	self.registerSignal()
	for {
		conn, err := listener.Accept()
		if err != nil {
			plog.Error(err)
			return
		}
		go self.handle(conn)
	}
}

func (self *ConsoleServer) registerSignal() {
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
	rpcServer *ConsoleRpcServer
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
		consoleServer = NewConsoleServer(serverConfig.Global.Host, serverConfig.Console.Port)
		stor, err := storage.NewStorage(serverConfig.Global.StorageType)
		if err != nil {
			panic(err)
		}
		nodeManager.importStorage(stor)
		// start loggers
		nodeManager.pipeline, err = pl.NewPipeline(&serverConfig.Console.Loggers)
		if err != nil {
			panic(err)
		}
		// for linelogger to send the last buffer
		go nodeManager.PeriodicTask()
		runtime.GOMAXPROCS(serverConfig.Global.Worker)
		nodeManager.initConsole()
		go nodeManager.PersistWatcher()
		go consoleServer.Listen()
		if nodeManager.stor.SupportWatcher() {
			nodeManager.rpcServer = newConsoleRpcServer()
			nodeManager.rpcServer.serve()
		}
	}
	return nodeManager
}

// Import storage.Nodes to NodeManager.Nodes
func (self *NodeManager) importStorage(stor storage.StorInterface) {
	stor.ImportNodes()
	self.RWlock.Lock()
	for k, v := range stor.GetNodes() {
		var node *Node
		if _, ok := self.Nodes[k]; !ok {
			node = NewNodeFromStor(v)
			node.init()
		} else {
			node.StorageNode = v
		}
		if node.StorageNode.Ondemand == false {
			node.logging = true
		} else {
			node.logging = false
		}
		self.Nodes[k] = node
	}
	self.RWlock.Unlock()
	nodeManager.stor = stor
}

// export NodeManager.Nodes to storage.Nodes
func (self *NodeManager) exportStorage() map[string]*storage.Node {
	storNodes := make(map[string]*storage.Node)
	self.RWlock.RLock()
	for k, v := range self.Nodes {
		storNodes[k] = v.StorageNode
	}
	self.RWlock.RUnlock()
	return storNodes
}

// start console if required
func (self *NodeManager) initConsole() {
	self.RWlock.RLock()
	for _, v := range self.Nodes {
		// node is a pointer, it's ok to init like this。
		// Can not use v dirrectly as the reference of v would be changed within the iteration. (tricky)
		node := v
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
				go node.restartMonitor()
				go node.startConsole()
				if err := common.TimeoutChan(node.GetReadyChan(),
					serverConfig.Console.TargetTimeout); err != nil {
					plog.ErrorNode(node.StorageNode.Name, err)
				}
				node.Release(false)
			}()
		}
	}
	self.RWlock.RUnlock()
}

func (self *NodeManager) ListNode() map[string][]map[string]string {
	nodes := make(map[string][]map[string]string)
	nodes["nodes"] = make([]map[string]string, 0)
	if !self.stor.SupportWatcher() {
		self.RWlock.RLock()
		for _, node := range self.Nodes {
			nodeMap := make(map[string]string)
			nodeMap["name"] = node.StorageNode.Name
			nodeMap["host"] = self.hostname
			nodeMap["state"] = STATUS_MAP[node.GetStatus()]
			nodes["nodes"] = append(nodes["nodes"], nodeMap)
		}
		self.RWlock.RUnlock()
	} else {
		nodeWithHost, err := self.stor.ListNodeWithHost()
		if err != nil {
			return nodes
		}
		endpoints, err := self.stor.GetVhosts()
		if err != nil || len(endpoints) == 0 {
			return nodes
		}
		statusMap := make(map[string]int)
		for vhost, config := range endpoints {
			rpcClient, err := newConsoleRpcClient(config.Host, vhost)
			if err != nil {
				plog.Error(fmt.Sprintf("Could not init RpcClient instance, error: %s", err))
				continue
			}
			rpcResult, err := rpcClient.ListNodesStatus()
			if err != nil {
				plog.Error(fmt.Sprintf("Could not get rpc result from host %s, Error: %s", config.Host, err.Error()))
				continue
			}
			for k, v := range rpcResult {
				statusMap[k] = v
			}
		}
		for node, vhost := range nodeWithHost {
			nodeMap := make(map[string]string)
			nodeMap["name"] = node
			nodeMap["host"] = vhost
			if status, ok := statusMap[node]; ok {
				nodeMap["state"] = STATUS_MAP[status]
			} else {
				nodeMap["state"] = STATUS_MAP[STATUS_NOTFOUND]
			}
			nodes["nodes"] = append(nodes["nodes"], nodeMap)
		}
	}
	return nodes
}

func (self *NodeManager) ShowNode(name string) (*Node, int, string) {
	var node *Node
	if !self.stor.SupportWatcher() {
		if !self.Exists(name) {
			return nil, http.StatusBadRequest, fmt.Sprintf("The node %s is not exist.", name)
		}
		self.RWlock.RLock()
		node = self.Nodes[name]
		self.RWlock.RUnlock()
		node.State = STATUS_MAP[node.GetStatus()]
	} else {
		nodeWithHost, err := self.stor.ListNodeWithHost()
		if err != nil {
			return nil, http.StatusInternalServerError, "Could not get host information, please check the storage connection"
		}
		if _, ok := nodeWithHost[name]; !ok {
			return nil, http.StatusBadRequest, fmt.Sprintf("Could not get host information for node %s", name)
		}
		endpoint, err := self.stor.GetEndpoint(nodeWithHost[name])
		if err != nil {
			return nil, http.StatusInternalServerError, "Could not get host information, please check the storage connection"
		}
		rpcClient, err := newConsoleRpcClient(endpoint.Host, nodeWithHost[name])
		if err != nil {
			return nil, http.StatusInternalServerError, err.Error()
		}
		pbNode, err := rpcClient.ShowNode(name)
		if err != nil {
			return nil, http.StatusInternalServerError, err.Error()
		}
		node = NewNodeFromProto(pbNode)
	}
	return node, http.StatusOK, ""
}

func (self *NodeManager) setConsoleState(nodes []string, state string) map[string]string {
	result := make(map[string]string)
	for _, v := range nodes {
		if v == "" {
			plog.Error("Skip this record as node name is not defined.")
			continue
		}
		self.RWlock.Lock()
		if !self.Exists(v) {
			msg := "Skip this node as it is not exist."
			plog.ErrorNode(v, msg)
			result[v] = msg
			self.RWlock.Unlock()
			continue
		}
		node := self.Nodes[v]
		self.RWlock.Unlock()
		if state == CONSOLE_ON {
			node.logging = true
			if node.GetStatus() != STATUS_CONNECTED {
				node.StartConsole()
				result[v] = common.RESULT_UPDATED
				continue
			}
			result[v] = common.RESULT_UNCHANGED
		} else if state == CONSOLE_OFF {
			node.logging = false
			if node.GetStatus() != STATUS_CONNECTED {
				result[v] = common.RESULT_UNCHANGED
				continue
			}
			if err := node.StopConsole(); err != nil {
				result[v] = err.Error()
				continue
			}
			result[v] = common.RESULT_UPDATED
		}
	}
	return result
}

func (self *NodeManager) SetConsoleState(nodes []string, state string) (map[string]string, error) {
	var node, vhost string
	var nodeList []string
	if !self.stor.SupportWatcher() {
		return self.setConsoleState(nodes, state), nil
	}
	nodeWithHost, err := self.stor.ListNodeWithHost()
	if err != nil {
		return nil, err
	}
	hostNodes := make(map[string][]string, 0)
	for _, node = range nodes {
		if _, ok := nodeWithHost[node]; !ok {
			plog.ErrorNode(node, "Skip this node as it is not exist.")
			continue
		}
		vhost = nodeWithHost[node]
		//if hostNodes[]
		hostNodes[vhost] = append(hostNodes[vhost], node)
	}
	result := make(map[string]string)
	for vhost, nodeList = range hostNodes {
		endpoint, err := self.stor.GetEndpoint(vhost)
		if err != nil {
			continue
		}
		rpcClient, err := newConsoleRpcClient(endpoint.Host, vhost)
		if err != nil {
			plog.Error(fmt.Sprintf("Could not init RpcClient instance, error: %s", err))
			continue
		}
		hostResult, err := rpcClient.SetConsoleState(nodeList, state)
		if err != nil {
			continue
		}
		for k, v := range hostResult {
			result[k] = v
		}
	}
	return result, nil
}

func (self *NodeManager) postNode(storNode *storage.Node) (int, string) {
	node := NewNodeFromStor(storNode)
	if err := node.Validate(); err != nil {
		return http.StatusBadRequest, err.Error()
	}
	self.RWlock.Lock()
	if self.Exists(node.StorageNode.Name) {
		self.RWlock.Unlock()
		return http.StatusConflict, fmt.Sprintf("The node name %s is already exist", node.StorageNode.Name)
	}
	node.SetStatus(STATUS_ENROLL)
	self.Nodes[node.StorageNode.Name] = node
	self.RWlock.Unlock()
	plog.InfoNode(node.StorageNode.Name, "Created.")
	if node.StorageNode.Ondemand == false {
		go node.restartMonitor()
		node.StartConsole()
	}
	return http.StatusAccepted, ""
}

func (self *NodeManager) PostNode(storNode *storage.Node) (int, string) {
	var err error
	if !self.stor.SupportWatcher() {
		code, msg := self.postNode(storNode)
		if msg != "" {
			return code, msg
		}
		err = self.NotifyPersist(nil, storage.ACTION_NIL)
	} else {
		err = self.NotifyPersist(storNode, storage.ACTION_PUT)
	}
	if err != nil {
		return http.StatusBadRequest, err.Error()
	}
	return http.StatusAccepted, ""
}

func (self *NodeManager) postNodes(storNodes []storage.Node, result map[string]string) {
	for i := 0; i < len(storNodes); i++ {
		if storNodes[i].Name == "" {
			plog.Error("Skip this record as node name is not defined.")
			continue
		}
		if storNodes[i].Driver == "" {
			msg := "Driver is not defined."
			plog.ErrorNode(storNodes[i].Name, msg)
			if result != nil {
				result[storNodes[i].Name] = msg
			}
			continue
		}
		node := NewNodeFromStor(&storNodes[i])
		if err := node.Validate(); err != nil {
			plog.ErrorNode(node.StorageNode.Name, err.Error())
			if result != nil {
				result[node.StorageNode.Name] = err.Error()
			}
			continue
		}
		self.RWlock.Lock()
		if self.Exists(node.StorageNode.Name) {
			msg := "Skip this node as node is exist."
			plog.ErrorNode(node.StorageNode.Name, msg)
			if result != nil {
				result[node.StorageNode.Name] = msg
			}
			self.RWlock.Unlock()
			continue
		}
		node.SetStatus(STATUS_ENROLL)
		self.Nodes[node.StorageNode.Name] = node
		self.RWlock.Unlock()
		if result != nil {
			result[node.StorageNode.Name] = common.RESULT_CREATED
		}
		if node.StorageNode.Ondemand == false {
			go node.restartMonitor()
			node.StartConsole()
		}
	}
}

func (self *NodeManager) PostNodes(storNodes map[string][]storage.Node) (map[string]string, error) {
	var err error
	result := make(map[string]string)
	if !self.stor.SupportWatcher() {
		self.postNodes(storNodes["nodes"], result)
		err = self.NotifyPersist(nil, storage.ACTION_NIL)
	} else {
		for _, v := range storNodes["nodes"] {
			result[v.Name] = common.RESULT_ACCEPTED
		}
		err = self.NotifyPersist(storNodes["nodes"], storage.ACTION_MULTIPUT)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *NodeManager) deleteNode(nodeName string) (int, string) {
	self.RWlock.Lock()
	node, ok := self.Nodes[nodeName]
	if !ok {
		self.RWlock.Unlock()
		return http.StatusBadRequest, fmt.Sprintf("Node %s is not exist", nodeName)
	}
	if node.StorageNode.Ondemand == false {
		close(node.reconnect)
	}
	if node.GetStatus() == STATUS_CONNECTED {
		if err := node.StopConsole(); err != nil {
			self.RWlock.Unlock()
			return http.StatusConflict, err.Error()
		}
	}
	delete(self.Nodes, nodeName)
	self.RWlock.Unlock()
	return http.StatusAccepted, ""
}

func (self *NodeManager) DeleteNode(nodeName string) (int, string) {
	var err error
	if !self.stor.SupportWatcher() {
		code, msg := self.deleteNode(nodeName)
		if msg != "" {
			return code, msg
		}
		err = self.NotifyPersist(nil, storage.ACTION_NIL)
	} else {
		err = self.NotifyPersist(nodeName, storage.ACTION_DEL)
	}
	if err != nil {
		return http.StatusBadRequest, err.Error()
	}
	return http.StatusAccepted, ""
}

func (self *NodeManager) deleteNodes(names []string, result map[string]string) {
	for _, v := range names {
		if v == "" {
			plog.Error("Skip this record as node name is not defined.")
			continue
		}
		self.RWlock.Lock()
		if !self.Exists(v) {
			msg := "Skip this node as node is not exist."
			plog.ErrorNode(v, msg)
			if result != nil {
				result[v] = msg
			}
			self.RWlock.Unlock()
			continue
		}
		node := self.Nodes[v]
		if node.StorageNode.Ondemand == false {
			close(node.reconnect)
		}
		if node.GetStatus() == STATUS_CONNECTED {
			if err := node.StopConsole(); err != nil {
				self.RWlock.Unlock()
				continue
			}
		}
		delete(self.Nodes, v)
		self.RWlock.Unlock()
		if result != nil {
			result[v] = common.RESULT_DELETED
		}
	}
}

func (self *NodeManager) DeleteNodes(names []string) (map[string]string, error) {
	var err error
	result := make(map[string]string)
	if !self.stor.SupportWatcher() {
		self.deleteNodes(names, result)
		err = self.NotifyPersist(nil, storage.ACTION_NIL)
	} else {
		for _, name := range names {
			result[name] = common.RESULT_ACCEPTED
		}
		err = self.NotifyPersist(names, storage.ACTION_MULTIDEL)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (self *NodeManager) Replay(name string) (string, int, string) {
	var content string
	var err error
	if !self.stor.SupportWatcher() {
		if !self.Exists(name) {
			return "", http.StatusBadRequest, fmt.Sprintf("The node %s is not exist.", name)
		}
		// TODO: make replaylines more flexible
		content, err = self.pipeline.Fetch(name, serverConfig.Console.ReplayLines)
		if err != nil {
			return "", http.StatusInternalServerError, err.Error()
		}
	} else {
		nodeWithHost, err := self.stor.ListNodeWithHost()
		if err != nil {
			return "", http.StatusInternalServerError, "Could not get host information, please check the storage connection"
		}
		if _, ok := nodeWithHost[name]; !ok {
			return "", http.StatusBadRequest, fmt.Sprintf("Could not get host information for node %s", name)
		}
		endpoint, err := self.stor.GetEndpoint(nodeWithHost[name])
		if err != nil {
			return "", http.StatusInternalServerError, "Could not get host information, please check the storage connection"
		}
		rpcClient, err := newConsoleRpcClient(endpoint.Host, nodeWithHost[name])
		if err != nil {
			return "", http.StatusInternalServerError, err.Error()
		}
		content, err = rpcClient.GetReplayContent(name)
		if err != nil {
			return "", http.StatusInternalServerError, err.Error()
		}
	}
	return content, http.StatusOK, ""
}

func (self *NodeManager) ListUser(name string) (ret map[string][]string, code int, msg string) {
	var node *Node
	var users []string
	var ok bool
	ret = make(map[string][]string)
	if !self.stor.SupportWatcher() {
		self.RWlock.RLock()
		if node, ok = self.Nodes[name]; !ok {
			self.RWlock.RUnlock()
			return nil, http.StatusBadRequest, fmt.Sprintf("The node %s is not exist.", name)
		}
		self.RWlock.RUnlock()
		defer func() {
			if r := recover(); r != nil {
				ret["users"] = make([]string, 0)
				code = http.StatusOK
				msg = ""
			}
		}()
		users = node.console.ListSessionUser()
	} else {
		nodeWithHost, err := self.stor.ListNodeWithHost()
		if err != nil {
			return nil, http.StatusInternalServerError, "Could not get host information, please check the storage connection"
		}
		if _, ok := nodeWithHost[name]; !ok {
			return nil, http.StatusBadRequest, fmt.Sprintf("Could not get host information for node %s", name)
		}
		endpoint, err := self.stor.GetEndpoint(nodeWithHost[name])
		if err != nil {
			return nil, http.StatusInternalServerError, "Could not get host information, please check the storage connection"
		}
		rpcClient, err := newConsoleRpcClient(endpoint.Host, nodeWithHost[name])
		if err != nil {
			return nil, http.StatusInternalServerError, err.Error()
		}
		users, err = rpcClient.ListSessionUser(name)
		if err != nil {
			return nil, http.StatusInternalServerError, err.Error()
		}
	}
	ret["users"] = users
	return ret, http.StatusOK, ""
}

func (self *NodeManager) NotifyPersist(node interface{}, action int) error {
	if !self.stor.SupportWatcher() {
		storNodes := self.exportStorage()
		self.stor.NotifyPersist(storNodes, storage.ACTION_NIL)
		return nil
	}
	return self.stor.NotifyPersist(node, action)
}

func (self *NodeManager) Exists(nodeName string) bool {
	if _, ok := self.Nodes[nodeName]; ok {
		return true
	}
	return false
}

func (self *NodeManager) PersistWatcher() {
	if !self.stor.SupportWatcher() {
		self.stor.PersistWatcher(nil)
		return
	}
	eventChan := make(chan interface{}, 128)
	go self.stor.PersistWatcher(eventChan)
	for {
		eventData, ok := (<-eventChan).(*storage.EventData)
		if !ok {
			continue
		}
		switch eventData.Action {
		case storage.ACTION_PUT:
			node, ok := eventData.Data.(*storage.Node)
			if !ok {
				plog.Error(common.ErrInvalidType)
				break
			}
			_, msg := self.postNode(node)
			if msg != "" {
				plog.ErrorNode(node.Name, msg)
			}
		case storage.ACTION_DEL:
			name, ok := eventData.Data.(string)
			if !ok {
				plog.Error(common.ErrInvalidType)
				break
			}
			_, msg := self.deleteNode(name)
			if msg != "" {
				plog.ErrorNode(name, msg)
			}
		}
	}
}

// for linelogger to send the last buffer
func (self *NodeManager) PeriodicTask() {
	if self.pipeline.Periodic == false {
		return
	}
	plog.Info("Starting peridic task")
	tick := time.Tick(common.PERIODIC_INTERVAL)
	for {
		<-tick
		current := time.Now()
		plog.Debug("Periodic task is running")
		readyList := make([]*ReadyBuffer, 0)
		self.RWlock.RLock()
		for k, v := range self.Nodes {
			console := v.console
			if console == nil {
				continue
			}
			if console.last.Buf != nil && console.last.Deadline.After(current) {
				readyBuf := &ReadyBuffer{node: k, last: console.last}
				readyList = append(readyList, readyBuf)
			}
		}
		self.RWlock.RUnlock()
		for _, readyBuf := range readyList {
			self.pipeline.PromptLast(readyBuf.node, readyBuf.last)
		}
	}
}
