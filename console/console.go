package console

import (
	"bytes"
	"fmt"
	"github.com/xcat2/goconserver/common"
	pl "github.com/xcat2/goconserver/console/pipeline"
	"github.com/xcat2/goconserver/plugins"
	"io"
	"net"
	"sync"
	"time"
)

const (
	CLIENT_CMD_EXIT   = '.'
	CLIENT_CMD_HELP   = '?'
	CLIENT_CMD_REPLAY = 'r'
	CLIENT_CMD_WHO    = 'w'
)

var (
	CLIENT_CMDS   = []byte{CLIENT_CMD_HELP, CLIENT_CMD_REPLAY, CLIENT_CMD_WHO}
	EXIT_SEQUENCE = [...]byte{'\x05', 'c', '.'} // ctrl-e, c
)

type Console struct {
	bufConn   map[net.Conn]chan []byte // build the map for the connection and the channel
	remoteIn  io.Writer
	remoteOut io.Reader
	session   plugins.ConsoleSession // interface for plugin
	running   chan bool
	node      *Node
	mutex     *sync.RWMutex
	last      *pl.RemainBuffer // the rest of the buffer that has not been emitted
}

func NewConsole(baseSession *plugins.BaseSession, node *Node) *Console {
	return &Console{remoteIn: baseSession.In,
		remoteOut: baseSession.Out,
		session:   baseSession.Session,
		node:      node,
		bufConn:   make(map[net.Conn]chan []byte),
		running:   make(chan bool, 0),
		mutex:     new(sync.RWMutex),
		last:      new(pl.RemainBuffer),
	}
}

// Accept connection from client
func (self *Console) Accept(conn net.Conn) {
	plog.DebugNode(self.node.StorageNode.Name, "Accept connection from client")
	self.mutex.Lock()
	self.bufConn[conn] = make(chan []byte)
	self.mutex.Unlock()
	go self.writeTarget(conn)
	self.writeClient(conn)
}

// Disconnect from client
func (self *Console) Disconnect(conn net.Conn) {
	var bufChan chan []byte
	var ok bool
	conn.Close()
	self.mutex.Lock()
	if bufChan, ok = self.bufConn[conn]; ok {
		close(bufChan)
		delete(self.bufConn, conn)
	}
	self.mutex.Unlock()
	// all of the client has been disconnected
	if len(self.bufConn) == 0 && self.node.logging == false {
		self.Close()
	}
}

func (self *Console) writeTarget(conn net.Conn) {
	plog.DebugNode(self.node.StorageNode.Name, "Create new connection to read message from client.")
	defer func() {
		plog.DebugNode(self.node.StorageNode.Name, "writeTarget goroutine quit")
		self.Disconnect(conn)
	}()
	for {
		self.mutex.RLock()
		if _, ok := self.bufConn[conn]; !ok {
			self.mutex.RUnlock()
			plog.ErrorNode(self.node.StorageNode.Name, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		self.mutex.RUnlock()
		n, err := common.Network.ReceiveInt(conn)
		if err != nil {
			plog.WarningNode(self.node.StorageNode.Name, fmt.Sprintf("Failed to receive message head from client. Error:%s.", err.Error()))
			return
		}
		b, err := common.Network.ReceiveBytes(conn, n)
		if err != nil {
			plog.WarningNode(self.node.StorageNode.Name, fmt.Sprintf("Failed to receive message from client. Error:%s.", err.Error()))
			return
		}
		if bytes.Equal(b, EXIT_SEQUENCE[0:]) {
			plog.InfoNode(self.node.StorageNode.Name, "Received exit signal from client")
			return
		}
		tmp := 0
		for n > 0 {
			count, err := self.remoteIn.Write(b[tmp:])
			if err != nil {
				plog.WarningNode(self.node.StorageNode.Name, fmt.Sprintf("Failed to send message to the remote server. Error:%s.", err.Error()))
				return
			}
			tmp += count
			n -= count
		}
	}
}

func (self *Console) writeClient(conn net.Conn) {
	plog.DebugNode(self.node.StorageNode.Name, "Create new connection to write message to client.")
	defer func() {
		plog.DebugNode(self.node.StorageNode.Name, "writeClient goroutine quit")
		self.Disconnect(conn)
	}()
	var bufChan chan []byte
	var ok bool
	clientTimeout := time.Duration(serverConfig.Console.ClientTimeout)
	welcome := fmt.Sprintf("goconserver(%s): Hello %s, welcome to the session of %s",
		time.Now().Format(common.RFC3339_SECOND), conn.RemoteAddr().String(), self.node.StorageNode.Name)
	err := common.Network.SendByteWithLengthTimeout(conn, []byte(welcome+"\r\n"), clientTimeout)
	if err != nil {
		plog.InfoNode(self.node.StorageNode.Name, fmt.Sprintf("Failed to send message to client. Error:%s", err.Error()))
		return
	}
	err = nodeManager.pipeline.Prompt(self.node.StorageNode.Name, welcome)
	if err != nil {
		plog.DebugNode(self.node.StorageNode.Name, err.Error())
	}
	for {
		self.mutex.RLock()
		if bufChan, ok = self.bufConn[conn]; !ok {
			self.mutex.RUnlock()
			plog.ErrorNode(self.node.StorageNode.Name, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		self.mutex.RUnlock()
		b := <-bufChan
		err = common.Network.SendByteWithLengthTimeout(conn, b, clientTimeout)
		if err != nil {
			plog.InfoNode(self.node.StorageNode.Name, fmt.Sprintf("Failed to send message to client. Error:%s", err.Error()))
			return
		}
	}
}

func (self *Console) readTarget() {
	plog.DebugNode(self.node.StorageNode.Name, "Read target session has been initialized.")
	defer func() {
		plog.DebugNode(self.node.StorageNode.Name, "readTarget goroutine quit")
		err := nodeManager.pipeline.Prompt(self.node.StorageNode.Name, "[goconserver disconnected]")
		if err != nil {
			plog.DebugNode(self.node.StorageNode.Name, err.Error())
		}
		self.Stop()
	}()
	var err error
	var n int
	b := make([]byte, common.BUF_SIZE)
	err = nodeManager.pipeline.Prompt(self.node.StorageNode.Name, "[goconserver connected]")
	if err != nil {
		plog.WarningNode(self.node.StorageNode.Name, err.Error())
	}
	for {
		select {
		case running := <-self.running:
			switch running {
			case false:
				plog.InfoNode(self.node.StorageNode.Name, "Stop readTarget session.")
				return
			}
		default:
		}
		n, err = self.remoteOut.Read(b)
		if err != nil {
			plog.WarningNode(self.node.StorageNode.Name, fmt.Sprintf("Could not receive message from remote. Error:", err.Error()))
			return
		}
		if n > 0 {
			self.writeClientChan(b[:n])
			nodeManager.pipeline.MakeRecord(self.node.StorageNode.Name, b[:n], self.last)
			if err != nil {
				plog.DebugNode(self.node.StorageNode.Name, fmt.Sprintf("Failed to log message. Error:%s", err.Error()))
			}
		}
	}
}

func (self *Console) writeClientChan(buf []byte) {
	b := make([]byte, len(buf))
	copy(b, buf)
	self.mutex.RLock()
	for k, v := range self.bufConn {
		select {
		case v <- b:
		case <-time.After(500 * time.Millisecond):
			plog.WarningNode(self.node.StorageNode.Name,
				fmt.Sprintf("Timeout for waiting client %s", k.RemoteAddr().String()))
		}
	}
	self.mutex.RUnlock()
}

func (self *Console) Start() {
	var err error
	defer func() {
		if err == nil {
			self.node.status = STATUS_AVAIABLE
		} else {
			self.node.status = STATUS_ERROR
		}
	}()
	plog.DebugNode(self.node.StorageNode.Name, "Start console session.")
	self.running = make(chan bool, 0)
	go self.readTarget()
	self.node.ready <- true
	self.node.status = STATUS_CONNECTED
	defer func() {
		// catch self.session nil pointer
		if r := recover(); r != nil {
			plog.WarningNode(self.node.StorageNode.Name, r)
		}
	}()
	if err = self.session.Wait(); err != nil {
		self.session.Close()
	} else {
		err = self.session.Close()
	}
}

// called from rest api to stop the console session
func (self *Console) Stop() {
	self.Close()
}

func (self *Console) ListSessionUser() []string {
	ret := make([]string, len(self.bufConn))
	i := 0
	self.mutex.RLock()
	for c, _ := range self.bufConn {
		ret[i] = c.RemoteAddr().String()
		i++
	}
	self.mutex.RUnlock()
	return ret
}

func (self *Console) Close() {
	if self.session == nil {
		// may be closed by the other thread
		return
	}
	self.mutex.Lock()
	// with lock check again
	if self.session == nil {
		self.mutex.Unlock()
		return
	}
	plog.DebugNode(self.node.StorageNode.Name, "Close console session.")
	for k, v := range self.bufConn {
		close(v)
		k.Close()
		delete(self.bufConn, k)
	}
	if self.running != nil {
		close(self.running)
		self.running = nil
	}
	if self.node.status == STATUS_CONNECTED {
		self.node.status = STATUS_AVAIABLE
	}
	self.node.console = nil
	self.session.Close()
	self.session = nil
	self.mutex.Unlock()
}
