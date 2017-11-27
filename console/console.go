package console

import (
	"bytes"
	"fmt"
	"github.com/chenglch/goconserver/common"
	"github.com/chenglch/goconserver/plugins"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	ExitSequence    = "\x05c." // ctrl-e, c
	CLIENT_CMD_EXIT = '.'
	CLIENT_CMD_HELP = '?'
	// TODO(chenglch): If client command need to access the service of server,
	// rest api could be used to implement this.

)

var (
	CLIENT_CMDS = []byte{CLIENT_CMD_HELP}
)

type Console struct {
	network   *common.Network
	bufConn   map[net.Conn]chan []byte // build the map for the connection and the channel
	remoteIn  io.Writer
	remoteOut io.Reader
	session   plugins.ConsoleSession // interface for plugin
	running   chan bool
	node      *Node
	mutex     *sync.Mutex
}

func NewConsole(baseSession *plugins.BaseSession, node *Node) *Console {
	return &Console{remoteIn: baseSession.In,
		remoteOut: baseSession.Out,
		session:   baseSession.Session,
		node:      node,
		network:   new(common.Network),
		bufConn:   make(map[net.Conn]chan []byte),
		running:   make(chan bool, 0),
		mutex:     new(sync.Mutex)}
}

// Accept connection from client
func (c *Console) Accept(conn net.Conn) {
	plog.DebugNode(c.node.StorageNode.Name, "Accept connection from client")
	c.bufConn[conn] = make(chan []byte)
	go c.writeTarget(conn)
	go c.writeClient(conn)
}

// Disconnect from client
func (c *Console) Disconnect(conn net.Conn) {
	var bufChan chan []byte
	var ok bool
	conn.Close()
	if bufChan, ok = c.bufConn[conn]; ok {
		c.mutex.Lock()
		if bufChan, ok = c.bufConn[conn]; ok {
			close(bufChan)
			delete(c.bufConn, conn)
		}
		c.mutex.Unlock()
	}
	// all of the client has been disconnected
	if len(c.bufConn) == 0 && c.node.StorageNode.Ondemand == true {
		c.Close()
	}
}

func (c *Console) writeTarget(conn net.Conn) {
	plog.DebugNode(c.node.StorageNode.Name, "Create new connection to read message from client.")
	defer func() {
		plog.DebugNode(c.node.StorageNode.Name, "writeTarget goruntine quit")
		c.Disconnect(conn)
	}()
	for {
		if _, ok := c.bufConn[conn]; !ok {
			plog.ErrorNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		n, err := c.network.ReceiveInt(conn)
		if err != nil {
			plog.WarningNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to receive message head from client. Error:%s.", err.Error()))
			return
		}
		b, err := c.network.ReceiveBytes(conn, n)
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
		plog.DebugNode(c.node.StorageNode.Name, "writeClient goruntine quit")
		c.Disconnect(conn)
	}()
	var bufChan chan []byte
	var ok bool
	clientTimeout := time.Duration(serverConfig.Console.ClientTimeout)
	welcome := fmt.Sprintf("goconserver(%s): Hello %s, welcome to the session of %s\r\n",
		time.Now().Format("2006-01-02 15:04:05"), conn.RemoteAddr().String(), c.node.StorageNode.Name)
	err := c.network.SendByteWithLengthTimeout(conn, []byte(welcome), clientTimeout)
	if err != nil {
		plog.WarningNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to send message to client. Error:%s", err.Error()))
		return
	}
	logFile := fmt.Sprintf("%s%c%s.log", serverConfig.Console.LogDir, filepath.Separator, c.node.StorageNode.Name)
	err = c.logger(logFile, []byte(welcome))
	if err != nil {
		plog.WarningNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to log message to %s. Error:%s", logFile, err.Error()))
	}
	for {
		if bufChan, ok = c.bufConn[conn]; !ok {
			plog.ErrorNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		b := <-bufChan
		err = c.network.SendByteWithLengthTimeout(conn, b, clientTimeout)
		if err != nil {
			plog.WarningNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to send message to client. Error:%s", err.Error()))
			return
		}
	}
}

func (c *Console) readTarget() {
	plog.DebugNode(c.node.StorageNode.Name, "Read target session has been initialized.")
	defer func() {
		plog.DebugNode(c.node.StorageNode.Name, "readTarget goruntine quit")
		c.Stop()
	}()
	var err error
	var n int
	b := make([]byte, 4096)
	logFile := fmt.Sprintf("%s%c%s.log", serverConfig.Console.LogDir, filepath.Separator, c.node.StorageNode.Name)
	msg := fmt.Sprintf("\nConnect to %s at %s\n\n", c.node.StorageNode.Name, time.Now().Format("2006-01-02 15:04:05"))
	err = c.logger(logFile, []byte(msg))
	if err != nil {
		plog.WarningNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to log message to %s. Error:%s", logFile, err.Error()))
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
			err = c.logger(logFile, b[:n])
			if err != nil {
				plog.WarningNode(c.node.StorageNode.Name, fmt.Sprintf("Failed to log message to %s. Error:%s", logFile, err.Error()))
			}
		}
	}
}

func (c *Console) writeClientChan(buf []byte) {
	b := make([]byte, len(buf))
	copy(b, buf)
	for _, v := range c.bufConn {
		v <- b
	}
}

func (c *Console) Start() {
	defer func() {
		c.node.status = STATUS_AVAIABLE
	}()
	plog.DebugNode(c.node.StorageNode.Name, "Start console session.")
	c.running = make(chan bool, 0)
	go c.readTarget()
	c.node.ready <- true
	c.node.logging = true
	c.node.status = STATUS_CONNECTED
	c.session.Wait()
	c.session.Close()
}

func (c *Console) Stop() {
	c.Close()
}

// close session from rest api
func (c *Console) Close() {
	plog.DebugNode(c.node.StorageNode.Name, "Close console session.")
	for k, v := range c.bufConn {
		c.mutex.Lock()
		if _, ok := c.bufConn[k]; ok {
			close(v)
		}
		k.Close()
		delete(c.bufConn, k)
		c.mutex.Unlock()
	}
	if c.running != nil {
		c.mutex.Lock()
		if c.running != nil {
			close(c.running)
			c.running = nil
		}
		c.mutex.Unlock()
	}
	c.node.status = STATUS_AVAIABLE
	c.node.console = nil
	c.session.Close()
}

func (c *Console) insertStamp(in []byte) ([]byte, error) {
	var buf bytes.Buffer
	var err error
	nextLine := false
	for i := 1; i < len(in); i++ {
		if in[i-1] == '\r' && in[i] == '\n' {
			_, err = buf.WriteString("\r\n[" + time.Now().Format("2006-01-02 15:04:05") + "]")
			if err != nil {
				return nil, err
			}
			i++
			nextLine = true
			continue
		}
		nextLine = false
		err = buf.WriteByte(in[i-1])
		if err != nil {
			return nil, err
		}
	}
	if nextLine == false {
		err = buf.WriteByte(in[len(in)-1])
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (c *Console) logger(path string, b []byte) error {
	if path != "" {
		var err error
		fd, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			plog.ErrorNode(c.node.StorageNode.Name, err)
			return err
		}
		defer fd.Close()
		if serverConfig.Console.LogTimestamp {
			b, err = c.insertStamp(b)
			if err != nil {
				plog.ErrorNode(c.node.StorageNode.Name, err)
				return err
			}
		}
		l := len(b)
		for l > 0 {
			n, err := fd.Write(b)
			if err != nil {
				plog.ErrorNode(c.node.StorageNode.Name, err)
				return err
			}
			l -= n
		}
	}
	return nil
}
