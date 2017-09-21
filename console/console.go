package console

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/chenglch/consoleserver/common"
	"github.com/chenglch/consoleserver/plugins"
)

const (
	ExitSequence = "\x05c." // ctrl-e, c
	MaxUint32    = 1<<32 - 1
)

type Console struct {
	network         *common.Network
	writeConns      chan net.Conn
	readConns       chan net.Conn
	bufConn         map[net.Conn]chan []byte // build the map for the connection and the channel
	remoteIn        io.Writer
	remoteOut       io.Reader
	session         plugins.ConsoleSession // interface for plugin
	stopWriteTarget chan bool
	stopReadTarget  chan bool
	stopWriteClient chan bool
	node            *Node
}

func NewConsole(baseSession *plugins.BaseSession, node *Node) *Console {
	return &Console{remoteIn: baseSession.In, remoteOut: baseSession.Out,
		session: baseSession.Session, node: node, network: new(common.Network),
		writeConns: make(chan net.Conn, 16), readConns: make(chan net.Conn, 16),
		bufConn: make(map[net.Conn]chan []byte), stopWriteClient: make(chan bool, 1),
		stopReadTarget: make(chan bool, 1), stopWriteTarget: make(chan bool, 1)}
}

// Accept connection from client
func (c *Console) Accept(conn net.Conn) {
	c.writeConns <- conn
	c.readConns <- conn
	c.bufConn[conn] = make(chan []byte)
}

// Disconnect from client
func (c *Console) Disconnect(conn net.Conn) {
	conn.Close()
	delete(c.bufConn, conn)
	// all of the client has been disconnected
	if len(c.bufConn) == 0 && c.node.Ondemand == true {
		c.Close()
		c.node.status = STATUS_AVAIABLE
	}
}

func (c *Console) innerWriteTarget(conn net.Conn) {
	plog.DebugNode(c.node.Name, "Create new connection to read message from client.")
	defer c.Disconnect(conn)
	for {
		if _, ok := c.bufConn[conn]; !ok {
			plog.ErrorNode(c.node.Name, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		n, err := c.network.ReceiveInt(conn)
		if err != nil {
			plog.WarningNode(c.node.Name, fmt.Sprintf("Failed to receive message head from client. Error:%s.", err.Error()))
			return
		}
		b, err := c.network.ReceiveBytes(conn, n)
		if err != nil {
			plog.WarningNode(c.node.Name, fmt.Sprintf("Failed to receive message from client. Error:%s.", err.Error()))
			return
		}
		if string(b) == ExitSequence {
			plog.InfoNode(c.node.Name, "Received exit signal from client")
			return
		}
		tmp := 0
		for n > 0 {
			count, err := c.remoteIn.Write(b[tmp:])
			if err != nil {
				plog.WarningNode(c.node.Name, fmt.Sprintf("Failed to send message to the remote server. Error:%s.", err.Error()))
				return
			}
			tmp += count
			n -= count
		}
	}
}

func (c *Console) writeTarget() {
	plog.DebugNode(c.node.Name, "Write target session has been initialized.")
	for {
		select {
		case conn := <-c.writeConns:
			go c.innerWriteTarget(conn)
		case <-c.stopWriteTarget:
			plog.DebugNode(c.node.Name, "Stop write target session.")
			return
		}
	}
}

func (c *Console) innerWriteClient(conn net.Conn) {
	plog.DebugNode(c.node.Name, "Create new connection to write message to client.")
	defer c.Disconnect(conn)
	socketTimeout := time.Duration(serverConfig.Console.SocketTimeout)
	for {
		if _, ok := c.bufConn[conn]; !ok {
			plog.ErrorNode(c.node.Name, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		b := <-c.bufConn[conn]
		err := c.network.SendByteWithLengthTimeout(conn, b, socketTimeout)
		if err != nil {
			plog.WarningNode(c.node.Name, fmt.Sprintf("Failed to send message to client. Error:%s", err.Error()))
			return
		}
	}
}

func (c *Console) writeClient() {
	for {
		select {
		case conn := <-c.readConns:
			go c.innerWriteClient(conn)
		case <-c.stopWriteClient:
			plog.InfoNode(c.node.Name, "Stop writeClient session.")
			return
		}
	}
}

func (c *Console) readTarget() {
	plog.DebugNode(c.node.Name, "Read target session has been initialized.")
	var err error
	var n int
	b := make([]byte, 4096)
	for {
		select {
		case <-c.stopReadTarget:
			plog.InfoNode(c.node.Name, "Stop readTarget session.")
			return
		default:
		}
		n, err = c.remoteOut.Read(b)
		if err != nil {
			plog.WarningNode(c.node.Name, fmt.Sprintf("Could not receive message from remote. Error:", err.Error()))
			return
		}
		if n > 0 {
			c.writeClientChan(b[:n])
			c.logger(fmt.Sprintf("%s%c%s.log", serverConfig.Console.LogDir, filepath.Separator, c.node.Name), b[:n])
		}
	}
}

func (c *Console) writeClientChan(buf []byte) {
	for _, v := range c.bufConn {
		v <- buf
	}
}

func (c *Console) Start() {
	var err error
	plog.DebugNode(c.node.Name, "Start console session.")
	_, err = common.GetTaskManager().Register(c.writeTarget)
	if err != nil {
		c.Close()
		return
	}
	_, err = common.GetTaskManager().Register(c.readTarget)
	if err != nil {
		c.Close()
		return
	}
	_, err = common.GetTaskManager().Register(c.writeClient)
	if err != nil {
		c.Close()
		return
	}
	if c.node.status != STATUS_CONNECTED {
		c.node.ready <- true
	}
	c.node.status = STATUS_CONNECTED
	c.session.Wait()
	c.session.Close()
}

func (c *Console) Stop() {
	c.Close()
}

// close session from rest api
func (c *Console) Close() {
	plog.DebugNode(c.node.Name, "Close console session.")
	c.stopReadTarget <- true // 1 buffer size so that Stop call from restapi will not be blocked
	c.stopWriteTarget <- true
	c.stopWriteClient <- true
	for k := range c.bufConn {
		k.Close()
		delete(c.bufConn, k)
	}
	c.session.Close()
}

func (c *Console) logger(path string, b []byte) error {
	if path != "" {
		var err error
		fd, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			plog.ErrorNode(c.node.Name, err)
			return err
		}
		l := len(b)
		for l > 0 {
			n, err := fd.Write(b)
			if err != nil {
				plog.ErrorNode(c.node.Name, err)
				return err
			}
			l -= n
		}
		fd.Close()
	}
	return nil
}
