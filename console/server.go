package console

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chenglch/consoleserver/common"
	"github.com/chenglch/consoleserver/plugins"
	"io/ioutil"
	"net"
	"os"
	"path"
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

	COMMAND_START_CONSOLE = "start_console"
)

var (
	plog           = common.GetLogger("github.com/chenglch/consoleserver/service")
	nodeManager    *NodeManager
	serverConfig   = common.GetServerConfig()
	nodeConfigFile string
	nodeBackupFile string
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
	rwLock   *sync.RWMutex
	reserve  int
}

func NewNode() *Node {
	node := new(Node)
	node.ready = make(chan bool, 0) // block client
	node.status = STATUS_AVAIABLE
	node.Ondemand = true
	node.rwLock = new(sync.RWMutex)
	node.reserve = common.TYPE_NO_LOCK
	return node
}

func (node *Node) Init() {
	node.rwLock = new(sync.RWMutex)
	node.reserve = common.TYPE_NO_LOCK
	node.ready = make(chan bool, 0) // block client
	node.status = STATUS_AVAIABLE
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

func (node *Node) restartConsole() {
	if _, ok := nodeManager.Nodes[node.Name]; !ok {
		plog.WarningNode(node.Name, "node has alrealy been removed.")
		return
	}
	if err := node.RequireLock(false); err != nil {
		plog.ErrorNode(node.Name, fmt.Sprintf("Could not start console, error: %s", err.Error()))
		return
	}
	if node.GetStatus() == STATUS_CONNECTED {
		node.Release(false)
		return
	}
	go node.StartConsole()
	if err := common.TimeoutChan(node.ready, serverConfig.Console.TargetTimeout); err != nil {
		plog.ErrorNode(node.Name, fmt.Sprintf("Could not start console, error: %s.", err))
		node.Release(false)
		return
	}
}

func (node *Node) StartConsole() {
	var consolePlugin plugins.ConsolePlugin
	var err error
	var baseSession *plugins.BaseSession
	consolePlugin, err = plugins.StartConsole(node.Driver, node.Name, node.Params)
	if err != nil {
		node.status = STATUS_ERROR
		node.ready <- false
		plog.ErrorNode(node.Name, fmt.Sprintf("Could not start console, wait 5 seconds and try again, error:%s", err.Error()))
		time.Sleep(time.Duration(5) * time.Second)
		go node.restartConsole()
		return
	}
	baseSession, err = consolePlugin.Start()
	if err != nil {
		node.status = STATUS_ERROR
		node.ready <- false
		plog.ErrorNode(node.Name, fmt.Sprintf("Could not start console, wait 5 seconds and try again, error:%s", err.Error()))
		time.Sleep(time.Duration(5) * time.Second)
		go node.restartConsole()
		return
	}
	console := NewConsole(baseSession, node)
	node.console = console
	console.Start()
	if node.Ondemand == false {
		plog.InfoNode(node.Name, "Start console again due to the ondemand setting.")
		time.Sleep(time.Duration(5) * time.Second)
		node.restartConsole()
	}
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
		c.SendInt(conn.(net.Conn), STATUS_ERROR)
		conn.(net.Conn).Close()
		return
	}
	if _, ok := nodeManager.Nodes[data["name"]]; !ok {
		plog.ErrorNode(data["name"], "Could not find this node.")
		c.SendInt(conn.(net.Conn), STATUS_ERROR)
		conn.(net.Conn).Close()
		return
	}
	node := nodeManager.Nodes[data["name"]]
	if data["command"] == COMMAND_START_CONSOLE {
		if node.status != STATUS_CONNECTED {
			if err := node.RequireLock(false); err != nil {
				plog.ErrorNode(node.Name, fmt.Sprintf("Could not start console, error: %s.", err))
				c.SendInt(conn.(net.Conn), STATUS_ERROR)
				conn.(net.Conn).Close()
				return
			}
			if node.status == STATUS_CONNECTED {
				node.Release(false)
			} else {
				go node.StartConsole()
				if err := common.TimeoutChan(node.ready, serverConfig.Console.TargetTimeout); err != nil {
					plog.ErrorNode(node.Name, fmt.Sprintf("Could not start console, error: %s.", err))
					node.Release(false)
					c.SendInt(conn.(net.Conn), STATUS_ERROR)
					conn.(net.Conn).Close()
					return
				}
			}
		}
		if node.status == STATUS_CONNECTED {
			plog.InfoNode(node.Name, "Register client connection successfully.")
			// reply success message to the client
			c.SendInt(conn.(net.Conn), STATUS_CONNECTED)
			node.console.Accept(conn.(net.Conn))
		} else {
			c.SendInt(conn.(net.Conn), STATUS_ERROR)
			conn.(net.Conn).Close()
		}
	}
}

func (c *ConsoleServer) Listen() {
	var listener net.Listener
	var err error
	if serverConfig.Global.SSLCACertFile != "" && serverConfig.Global.SSLKeyFile != "" && serverConfig.Global.SSLCertFile != "" {
		tlsConfig := common.LoadServerTlsConfig(serverConfig.Global.SSLCertFile,
			serverConfig.Global.SSLKeyFile, serverConfig.Global.SSLCACertFile)
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
	Nodes       map[string]*Node
	RWlock      *sync.RWMutex
	persistence uint32 // 0 no pending data, 1 has pending data
	pending     chan bool
}

func GetNodeManager() *NodeManager {
	if nodeManager == nil {
		nodeManager = new(NodeManager)
		nodeManager.Nodes = make(map[string]*Node)
		nodeManager.persistence = 0
		nodeManager.pending = make(chan bool, 1) // make it non-block
		nodeManager.RWlock = new(sync.RWMutex)
		consoleServer := NewConsoleServer(serverConfig.Global.Host, serverConfig.Console.Port)
		nodeManager.importNodes()
		runtime.GOMAXPROCS(serverConfig.Global.Worker)
		nodeManager.initNodes()
		go nodeManager.doPersist()
		go consoleServer.Listen()
	}
	return nodeManager
}

func (m *NodeManager) initNodes() {
	for _, v := range nodeManager.Nodes {
		node := v
		node.Init()
		if node.Ondemand == false {
			go func() {
				if err := node.RequireLock(false); err != nil {
					plog.WarningNode(node.Name, "Conflict while starting console.")
					return
				}
				if node.status == STATUS_CONNECTED {
					plog.WarningNode(node.Name, "Console has already been started.")
					node.Release(false)
					return
				}
				go node.StartConsole()
				if err := common.TimeoutChan(node.GetReadyChan(),
					serverConfig.Console.TargetTimeout); err != nil {
					plog.ErrorNode(node.Name, err)
				}
				node.Release(false)
			}()
		}
	}
}

func (m *NodeManager) Save() {
	var data []byte
	var err error
	m.RWlock.Lock()
	if data, err = json.Marshal(m.Nodes); err != nil {
		m.RWlock.Unlock()
		plog.Error(fmt.Sprintf("Could not Marshal the node map: %s.", err))
		panic(err)
	}
	m.RWlock.Unlock()
	nodeConfigFile = path.Join(serverConfig.Console.DataDir, "nodes.json")
	nodeBackupFile = path.Join(serverConfig.Console.DataDir, "nodes.json.bak")
	if ok, _ := common.PathExists(nodeConfigFile); ok {
		_, err = common.CopyFile(nodeBackupFile, nodeConfigFile)
		if err != nil {
			plog.Error(fmt.Sprintf("Unexpected error: %s, exit.", err))
			panic(err)
		}
	}
	err = common.WriteJsonFile(nodeConfigFile, data)
	if err != nil {
		plog.Error(fmt.Sprintf("Unexpected error: %s, exit.", err))
		panic(err)
	}
	go func() {
		_, err = common.CopyFile(nodeBackupFile, nodeConfigFile)
		if err != nil {
			plog.Error(fmt.Sprintf("Unexpected error: %s, exit.", err))
		}
	}()
}

func (m *NodeManager) importNodes() {
	nodeConfigFile = path.Join(serverConfig.Console.DataDir, "nodes.json")
	useBackup := false
	if ok, _ := common.PathExists(nodeConfigFile); ok {
		bytes, err := ioutil.ReadFile(nodeConfigFile)
		if err != nil {
			plog.Error(fmt.Sprintf("Could not read node configration file %s.", nodeConfigFile))
			useBackup = true
		}
		if err := json.Unmarshal(bytes, &nodeManager.Nodes); err != nil {
			plog.Error(fmt.Sprintf("Could not parse node configration file %s.", nodeConfigFile))
			useBackup = true
		}
	} else {
		useBackup = true
	}
	if !useBackup {
		return
	}
	nodeBackupFile = path.Join(serverConfig.Console.DataDir, "nodes.json.bak")
	if ok, _ := common.PathExists(nodeBackupFile); ok {
		plog.Info(fmt.Sprintf("Trying to load node bakup file %s.", nodeBackupFile))
		bytes, err := ioutil.ReadFile(nodeBackupFile)
		if err != nil {
			plog.Error(fmt.Sprintf("Could not read nonde backup file %s.", nodeBackupFile))
			return
		}
		if err := json.Unmarshal(bytes, &nodeManager.Nodes); err != nil {
			plog.Error(fmt.Sprintf("Could not parse node backup file %s.", nodeBackupFile))
			return
		}
		go func() {
			// as primary file can not be loaded, copy it from backup file
			_, err = common.CopyFile(nodeConfigFile, nodeBackupFile)
			if err != nil {
				plog.Error(fmt.Sprintf("Unexpected error: %s, exit.", err))
				panic(err)
			}
		}()
	}
}

func (m *NodeManager) MakePersist() {
	common.Notify(m.pending, &m.persistence, 1)
}

// a separate thread to save the data, avoid of frequent IO
func (m *NodeManager) doPersist() {
	common.Wait(m.pending, &m.persistence, 0, m.Save)
}

func (m *NodeManager) Exists(node string) bool {
	if _, ok := m.Nodes[node]; ok {
		return true
	}
	return false
}
