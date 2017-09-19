package console

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/chenglch/consoleserver/common"
)

const (
	ExitSequence = "\x05c." // ctrl-e, c
	MaxUint32    = 1<<32 - 1
)

var (
	plog         = common.GetLogger("github.com/chenglch/consoleserver/console")
	serverConfig *common.ServerConfig
)

func init() {
	serverConfig = common.GetServerConfig()
}

type ConsoleSession interface {
	Wait() error
	Close() error
	Start() (*Console, error)
}

type Console struct {
	network         *common.Network
	writeConns      chan net.Conn
	readConns       chan net.Conn
	bufConn         map[net.Conn]chan []byte // build the map for the connection and the channel
	remoteIn        io.Writer
	remoteOut       io.Reader
	sess            ConsoleSession // interface for plugin
	stopWriteTarget chan bool
	stopReadTarget  chan bool
	stopWriteClient chan bool
	node            string // node name
}

func NewConsole(remoteIn io.Writer, remoteOut io.Reader, sess ConsoleSession, node string) *Console {
	return &Console{remoteIn: remoteIn, remoteOut: remoteOut,
		sess: sess, node: node, network: new(common.Network),
		writeConns: make(chan net.Conn, 16), readConns: make(chan net.Conn, 16),
		bufConn: make(map[net.Conn]chan []byte), stopWriteClient: make(chan bool, 1),
		stopReadTarget: make(chan bool, 1), stopWriteTarget: make(chan bool, 1)}
}

func (c *Console) AddConn(conn net.Conn) {
	c.writeConns <- conn
	c.readConns <- conn
	c.bufConn[conn] = make(chan []byte)
}

func (c *Console) innerWriteTarget(conn net.Conn) {
	defer delete(c.bufConn, conn)
	defer conn.Close()
	for {
		if _, ok := c.bufConn[conn]; !ok {
			plog.ErrorNode(c.node, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		n, err := c.network.ReceiveInt(conn)
		if err != nil {
			plog.WarningNode(c.node, fmt.Sprintf("Failed to receive message head from client. Error:%s.", err.Error))
			return
		}
		b, err := c.network.ReceiveBytes(conn, n)
		if err != nil {
			plog.ErrorNode(c.node, fmt.Sprintf("Failed to receive message from client. Error:%s.", err.Error))
			return
		}
		if string(b) == ExitSequence {
			plog.WarningNode(c.node, "Received exit signal from client")
			return
		}
		tmp := 0
		for n > 0 {
			count, err := c.remoteIn.Write(b[tmp:])
			if err != nil {
				plog.ErrorNode(c.node, fmt.Sprintf("Failed to send message to the remote server. Error:%s.", err.Error()))
				return
			}
			tmp += count
			n -= count
		}
	}
}

func (c *Console) writeTarget() {
	plog.InfoNode(c.node, "Write target session has been initialized.")
	for {
		select {
		case conn := <-c.writeConns:
			go c.innerWriteTarget(conn)
		case <-c.stopWriteTarget:
			plog.InfoNode(c.node, "Stop write target session.")
			return
		}
	}
}

func (c *Console) innerWriteClient(conn net.Conn) {
	defer delete(c.bufConn, conn)
	defer conn.Close()
	socketTimeout := time.Duration(serverConfig.Console.SocketTimeout)
	for {
		if _, ok := c.bufConn[conn]; !ok {
			plog.ErrorNode(c.node, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		b := <-c.bufConn[conn]
		err := c.network.SendByteWithLengthTimeout(conn, b, socketTimeout)
		if err != nil {
			plog.ErrorNode(c.node, fmt.Sprintf("Failed to send message to client. Error:%s", err.Error()))
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
			plog.InfoNode(c.node, "Stop writeClient session.")
			return
		}
	}
}

func (c *Console) readTarget() {
	plog.InfoNode(c.node, "Read target session has been initialized.")
	var err error
	var n int
	b := make([]byte, 4096)
	for {
		select {
		case <-c.stopReadTarget:
			plog.InfoNode(c.node, "Stop readTarget session.")
			return
		default:
		}
		n, err = c.remoteOut.Read(b)
		if err != nil {
			plog.ErrorNode(c.node, err.Error())
			return
		}
		if n > 0 {
			c.writeClientChan(b[:n])
			c.logger(fmt.Sprintf("%s%c%s.log", serverConfig.Console.LogDir, filepath.Separator, c.node), b[:n])
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
	c.sess.Wait()
	c.sess.Close()
}

func (c *Console) Stop() {
	c.Close()
}

// close session from rest api
func (c *Console) Close() {
	c.stopReadTarget <- true // 1 buffer size so that Stop call from restapi will not be blocked
	c.stopWriteTarget <- true
	c.stopWriteClient <- true
	for k := range c.bufConn {
		k.Close()
		delete(c.bufConn, k)
	}
	c.sess.Close()
}

func (c *Console) logger(path string, b []byte) error {
	if path != "" {
		var err error
		fd, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			plog.ErrorNode(c.node, err)
			return err
		}
		l := len(b)
		for l > 0 {
			n, err := fd.Write(b)
			if err != nil {
				plog.ErrorNode(c.node, err)
				return err
			}
			l -= n
		}
		fd.Close()
	}
	return nil
}
