package console

import (
	"fmt"
	"github.com/chenglch/goconserver/common"
	"github.com/chenglch/goconserver/plugins"
	"io"
	"net"
	"sync"
	"time"
)

const (
	ExitSequence      = "\x05c." // ctrl-e, c
	CLIENT_CMD_EXIT   = '.'
	CLIENT_CMD_HELP   = '?'
	CLIENT_CMD_REPLAY = 'r'
	CLIENT_CMD_WHO    = 'w'
)

var (
	CLIENT_CMDS = []byte{CLIENT_CMD_HELP, CLIENT_CMD_REPLAY, CLIENT_CMD_WHO}
)

type Console struct {
	bufConn   map[net.Conn]chan []byte // build the map for the connection and the channel
	remoteIn  io.Writer
	remoteOut io.Reader
	session   plugins.ConsoleSession // interface for plugin
	running   chan bool
	node      *Node
	mutex     *sync.RWMutex
	last      *[]byte // the rest of the buffer that has not been emitted
}

func NewConsole(baseSession *plugins.BaseSession, node *Node) *Console {
	return &Console{remoteIn: baseSession.In,
		remoteOut: baseSession.Out,
		session:   baseSession.Session,
		node:      node,
		bufConn:   make(map[net.Conn]chan []byte),
		running:   make(chan bool, 0),
		mutex:     new(sync.RWMutex),
		last:      new([]byte),
	}
}

// Accept connection from client
func (c *Console) Accept(conn net.Conn) {
	plog.DebugNode(c.node.StorageNode.Name, "Accept connection from client")
	c.mutex.Lock()
	c.bufConn[conn] = make(chan []byte)
	c.mutex.Unlock()
	go c.writeTarget(conn)
	go c.writeClient(conn)
}

// Disconnect from client
func (c *Console) Disconnect(conn net.Conn) {
	var bufChan chan []byte
	var ok bool
	conn.Close()
	c.mutex.Lock()
	if bufChan, ok = c.bufConn[conn]; ok {
		close(bufChan)
		delete(c.bufConn, conn)
	}
	c.mutex.Unlock()
	// all of the client has been disconnected
	if len(c.bufConn) == 0 && c.node.logging == false {
		c.Close()
	}
}

func (c *Console) writeTarget(conn net.Conn) {
	plog.DebugNode(c.node.StorageNode.Name, "Create new connection to read message from client.")
	defer func() {
		plog.DebugNode(c.node.StorageNode.Name, "writeTarget goroutine quit")
		c.Disconnect(conn)
	}()
	for {
		c.mutex.RLock()
		if _, ok := c.bufConn[conn]; !ok {
			c.mutex.RUnlock()
			plog.ErrorNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		c.mutex.RUnlock()
		n, err := common.Network.ReceiveInt(conn)
		if err != nil {
			plog.WarningNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to receive message head from client. Error:%s.", err.Error()))
			return
		}
		b, err := common.Network.ReceiveBytes(conn, n)
		if err != nil {
			plog.WarningNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to receive message from client. Error:%s.", err.Error()))
			return
		}
		if string(b) == ExitSequence {
			plog.InfoNode(c.node.StorageNode.Name, "Received exit signal from client")
			return
		}
		tmp := 0
		for n > 0 {
			count, err := c.remoteIn.Write(b[tmp:])
			if err != nil {
				plog.WarningNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to send message to the remote server. Error:%s.", err.Error()))
				return
			}
			tmp += count
			n -= count
		}
	}
}

func (c *Console) writeClient(conn net.Conn) {
	plog.DebugNode(c.node.StorageNode.Name, "Create new connection to write message to client.")
	defer func() {
		plog.DebugNode(c.node.StorageNode.Name, "writeClient goroutine quit")
		c.Disconnect(conn)
	}()
	var bufChan chan []byte
	var ok bool
	clientTimeout := time.Duration(serverConfig.Console.ClientTimeout)
	welcome := fmt.Sprintf("goconserver(%s): Hello %s, welcome to the session of %s\r\n",
		time.Now().Format("2006-01-02 15:04:05"), conn.RemoteAddr().String(), c.node.StorageNode.Name)
	err := common.Network.SendByteWithLengthTimeout(conn, []byte(welcome), clientTimeout)
	if err != nil {
		plog.InfoNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to send message to client. Error:%s", err.Error()))
		return
	}
	err = nodeManager.pipeline.Prompt(c.node.StorageNode.Name, welcome)
	if err != nil {
		plog.DebugNode(c.node.StorageNode.Name, err.Error())
	}
	for {
		c.mutex.RLock()
		if bufChan, ok = c.bufConn[conn]; !ok {
			c.mutex.RUnlock()
			plog.ErrorNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		c.mutex.RUnlock()
		b := <-bufChan
		err = common.Network.SendByteWithLengthTimeout(conn, b, clientTimeout)
		if err != nil {
			plog.InfoNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to send message to client. Error:%s", err.Error()))
			return
		}
	}
}

func (c *Console) readTarget() {
	plog.DebugNode(c.node.StorageNode.Name, "Read target session has been initialized.")
	defer func() {
		plog.DebugNode(c.node.StorageNode.Name, "readTarget goroutine quit")
		err := nodeManager.pipeline.Prompt(c.node.StorageNode.Name, "[goconserver disconnected]")
		if err != nil {
			plog.DebugNode(c.node.StorageNode.Name, err.Error())
		}
		c.Stop()
	}()
	var err error
	var n int
	b := make([]byte, common.BUF_SIZE)
	err = nodeManager.pipeline.Prompt(c.node.StorageNode.Name, "[goconserver connected]")
	if err != nil {
		plog.WarningNode(c.node.StorageNode.Name, err.Error())
	}
	for {
		select {
		case running := <-c.running:
			switch running {
			case false:
				plog.InfoNode(c.node.StorageNode.Name, "Stop readTarget session.")
				return
			}
		default:
		}
		n, err = c.remoteOut.Read(b)
		if err != nil {
			plog.WarningNode(c.node.StorageNode.Name, fmt.Sprintf("Could not receive message from remote. Error:", err.Error()))
			return
		}
		if n > 0 {
			c.writeClientChan(b[:n])
			nodeManager.pipeline.MakeRecord(c.node.StorageNode.Name, b[:n], c.last)
			if err != nil {
				plog.DebugNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to log message. Error:%s", err.Error()))
			}
		}
	}
}

func (c *Console) writeClientChan(buf []byte) {
	b := make([]byte, len(buf))
	copy(b, buf)
	c.mutex.RLock()
	for _, v := range c.bufConn {
		v <- b
	}
	c.mutex.RUnlock()
}

func (c *Console) Start() {
	defer func() {
		c.node.status = STATUS_AVAIABLE
	}()
	plog.DebugNode(c.node.StorageNode.Name, "Start console session.")
	c.running = make(chan bool, 0)
	go c.readTarget()
	c.node.ready <- true
	c.node.status = STATUS_CONNECTED
	defer func() {
		// catch c.session nil pointer
		if r := recover(); r != nil {
			plog.WarningNode(c.node.StorageNode.Name, r)
		}
	}()
	c.session.Wait()
	c.session.Close()
}

// called from rest api to stop the console session
func (c *Console) Stop() {
	c.Close()
}

func (c *Console) ListSessionUser() []string {
	ret := make([]string, len(c.bufConn))
	i := 0
	c.mutex.RLock()
	for c, _ := range c.bufConn {
		ret[i] = c.RemoteAddr().String()
		i++
	}
	c.mutex.RUnlock()
	return ret
}

func (c *Console) Close() {
	if c.session == nil {
		// may be closed by the other thread
		return
	}
	c.mutex.Lock()
	// with lock check again
	if c.session == nil {
		c.mutex.Unlock()
		return
	}
	plog.DebugNode(c.node.StorageNode.Name, "Close console session.")
	for k, v := range c.bufConn {
		close(v)
		k.Close()
		delete(c.bufConn, k)
	}
	if c.running != nil {
		close(c.running)
		c.running = nil
	}
	c.node.status = STATUS_AVAIABLE
	c.node.console = nil
	c.session.Close()
	c.session = nil
	c.mutex.Unlock()
}
