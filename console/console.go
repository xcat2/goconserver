package console

import (
	"fmt"
	"io"
	"net"
	"os"

	"github.com/chenglch/consoleserver/common"
)

const (
	ExitSequence = "\x05c." // ctrl-e, c
	MaxUint32    = 1<<32 - 1
)

var (
	plog *common.Logger
)

func init() {
	plog = common.GetLogger("github.com/chenglch/consoleserver/console")
}

type ConsoleSession interface {
	Wait() error
	Close() error
	Start() (*Console, error)
}

type Console struct {
	network             *common.Network
	writeConns          chan net.Conn
	readConns           chan net.Conn
	bufConn             map[net.Conn]chan []byte // build the map for the connection and the channel
	remoteIn            io.Writer
	remoteOut           io.Reader
	sess                ConsoleSession // interface for plugin
	writeTargetTaskChan chan bool
	readTaskChan        chan bool
	writeClientTaskChan chan bool
	node                string // node name
}

func NewConsole(remoteIn io.Writer, remoteOut io.Reader, sess ConsoleSession, node string) *Console {
	return &Console{remoteIn: remoteIn, remoteOut: remoteOut,
		sess: sess, node: node, network: new(common.Network),
		writeConns: make(chan net.Conn, 16), readConns: make(chan net.Conn, 16),
		bufConn: make(map[net.Conn]chan []byte), writeClientTaskChan: make(chan bool, 0),
		readTaskChan: make(chan bool, 0), writeTargetTaskChan: make(chan bool, 0)}
}

func (c *Console) AddConn(conn net.Conn) {
	c.writeConns <- conn
	c.readConns <- conn
	c.bufConn[conn] = make(chan []byte)
}

func (c *Console) innerWrite(conn net.Conn) {
	defer delete(c.bufConn, conn)
	defer conn.Close()
	for {
		if _, ok := c.bufConn[conn]; !ok {
			plog.ErrorNode(c.node, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		n, err := c.network.ReceiveInt(conn)
		if err != nil {
			plog.ErrorNode(c.node, fmt.Sprintf("Failed to receive message head from client. Error:%s.", err.Error()))
			return
		}
		b, err := c.network.ReceiveBytes(conn, n)
		if err != nil {
			plog.ErrorNode(c.node, fmt.Sprintf("Failed to receive message from client. Error:%s.", err.Error()))
			return
		}
		if string(b) == ExitSequence {
			plog.ErrorNode(c.node, "Received exit signal from client")
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
	plog.InfoNode(c.node, "writeTarget called")
	for {
		select {
		case conn := <-c.writeConns:
			go c.innerWrite(conn)
		case <-c.writeTargetTaskChan:
			plog.InfoNode(c.node, "Exit write target rountine.")
			return
		}
	}
}

func (c *Console) innerWriteToClient(conn net.Conn) {
	defer delete(c.bufConn, conn)
	defer conn.Close()
	// use local variable to avoid of the changes of the member variable
	for {
		if _, ok := c.bufConn[conn]; !ok {
			plog.ErrorNode(c.node, fmt.Sprintf("Failed to find the connection from bufConn, the connection may be closed."))
			return
		}
		b := <-c.bufConn[conn]
		err := c.network.SendByteWithLength(conn, b)
		if err != nil {
			plog.ErrorNode(c.node, fmt.Sprintf("Failed to send message to client. Error:%s", err.Error()))
			return
		}
	}
}

func (c *Console) writeToClient() {
	for {
		select {
		case conn := <-c.readConns:
			go c.innerWriteToClient(conn)
		case <-c.writeClientTaskChan:
			plog.InfoNode(c.node, "Exit writeToClient rountine.")
			return
		}
	}
}

func (c *Console) readTarget() {
	plog.InfoNode(c.node, "readTarget called")
	var err error
	var n int
	b := make([]byte, 4096)
	for {
		select {
		case <-c.readTaskChan:
			plog.InfoNode(c.node, "Exit readTarget rountine.")
			return
		default:
		}
		n, err = c.remoteOut.Read(b)
		if err != nil {
			plog.ErrorNode(c.node, err.Error())
			return
		}
		if n > 0 {
			c.sendBufToClientChannel(b[:n])
			c.logger(fmt.Sprintf("logs/%s.log", c.node), b[:n])
		}
	}
}

func (c *Console) sendBufToClientChannel(buf []byte) {
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
	_, err = common.GetTaskManager().Register(c.writeToClient)
	if err != nil {
		c.Close()
		return
	}
	c.sess.Wait()
	c.sess.Close()
}

// close session from rest api
func (c *Console) Close() {
	c.readTaskChan <- true
	c.writeTargetTaskChan <- true
	c.writeClientTaskChan <- true
	for k := range c.bufConn {
		delete(c.bufConn, k)
	}
	c.sess.Close()
}

func (c *Console) logger(path string, b []byte) error {
	if path != "" {
		var err error
		fd, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		l := len(b)
		for l > 0 {
			n, err := fd.Write(b)
			if err != nil {
				return err
			}
			l -= n
		}
		fd.Close()
	}
	return nil
}
