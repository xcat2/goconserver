package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/xcat2/goconserver/common"
	"io"
	"net"
	"time"
)

const (
	ACTION_SESSION_ERROR    = 0
	ACTION_SESSION_START    = 1
	ACTION_SESSION_DROP     = 2
	ACTION_SESSION_REDIRECT = 3
	ACTION_SESSION_OK       = 4
	ACTION_SESSION_RETRY    = 5
)

type ProtoMessage struct {
	Action      int    `json:"action"`
	Node        string `json:"node,omitempty"`
	Msg         string `json:"msg,omitempty"`
	Host        string `json:"host,omitempty"`
	ConsolePort string `json:"console_port,omitempty"`
	ApiPort     string `json:"api_port,omitempty"`
}

func clientHandshake(conn net.Conn, node string) (*ProtoMessage, error) {
	m := &ProtoMessage{Action: ACTION_SESSION_START, Node: node}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	consoleTimeout := time.Duration(clientConfig.ConsoleTimeout)
	err = common.Network.SendByteWithLengthTimeout(conn, b, consoleTimeout)
	if err != nil {
		return nil, err
	}
	n, err := common.Network.ReceiveIntTimeout(conn, consoleTimeout)
	if err != nil {
		return nil, err
	}
	b, err = common.Network.ReceiveBytesTimeout(conn, n, consoleTimeout)
	if err != nil && err != io.EOF {
		return nil, err
	}
	err = json.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func sendProtoMessage(conn net.Conn, msg string, action int, timeout time.Duration) error {
	m := new(ProtoMessage)
	m.Msg = msg
	m.Action = action
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	err = common.Network.SendByteWithLengthTimeout(conn.(net.Conn), b, timeout)
	if err != nil {
		return err
	}
	return nil
}

func initConsoleSessionClient(node string, host string, consolePort string) (*ConsoleClient, net.Conn, error) {
	client := NewConsoleClient(host, consolePort)
	conn, err := client.Connect()
	if err != nil {
		return nil, nil, err
	}
	m, err := clientHandshake(conn, node)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	switch m.Action {
	case ACTION_SESSION_DROP:
		return nil, nil, errors.New(m.Msg)
	case ACTION_SESSION_REDIRECT:
		conn.Close()
		return initConsoleSessionClient(node, m.Host, m.ConsolePort)
	case ACTION_SESSION_OK:
		return client, conn, nil
	case ACTION_SESSION_RETRY:
		conn.Close()
		return client, nil, nil
	}
	return client, conn, nil
}

func redirect(conn net.Conn, node string) error {
	var err error
	nodeWithHost, err := nodeManager.stor.ListNodeWithHost()
	if err != nil {
		plog.ErrorNode(node, err)
		return err
	}
	clientTimeout := time.Duration(serverConfig.Console.ClientTimeout)
	if _, ok := nodeWithHost[node]; !ok {
		plog.ErrorNode(node, "Could not find this node.")
		err = sendProtoMessage(conn, common.ErrNodeNotExist.Error(), ACTION_SESSION_DROP, clientTimeout)
		if err != nil {
			plog.ErrorNode(node, err)
		}
		return common.ErrNodeNotExist
	}
	host := nodeWithHost[node]
	endpoint, err := nodeManager.stor.GetEndpoint(host)
	if err != nil {
		plog.ErrorNode(node, err)
		return err
	}
	msg := ProtoMessage{Action: ACTION_SESSION_REDIRECT, Host: endpoint.Host, ConsolePort: endpoint.ConsolePort, ApiPort: endpoint.ApiPort}
	b, err := json.Marshal(msg)
	if err != nil {
		plog.ErrorNode(node, err)
		return err
	}
	err = common.Network.SendByteWithLengthTimeout(conn, b, clientTimeout)
	if err != nil {
		plog.ErrorNode(node, err)
		return err
	}
	plog.InfoNode(node, fmt.Sprintf("Redirect the session to %s", host))
	return nil
}

func serverHandshake(conn net.Conn) (*Node, error) {
	var name string
	var m *ProtoMessage
	var node *Node
	var ok bool
	clientTimeout := time.Duration(serverConfig.Console.ClientTimeout)
	n, err := common.Network.ReceiveIntTimeout(conn, clientTimeout)
	if err != nil {
		conn.Close()
		plog.Error(err)
		return nil, err
	}
	b, err := common.Network.ReceiveBytesTimeout(conn, n, clientTimeout)
	if err != nil {
		conn.Close()
		plog.Error(err)
		return nil, err
	}
	plog.Debug(fmt.Sprintf("Receive connection from client: %s", string(b)))
	if err := json.Unmarshal(b, &m); err != nil {
		defer conn.Close()
		tempErr := err
		plog.Error(err)
		err = sendProtoMessage(conn, err.Error(), ACTION_SESSION_ERROR, clientTimeout)
		if err != nil {
			plog.ErrorNode(name, err)
			return nil, err
		}
		return nil, tempErr
	}
	if m.Action != ACTION_SESSION_START || m.Node == "" {
		conn.Close()
		return nil, common.ErrConnection
	}
	name = m.Node
	nodeManager.RWlock.RLock()
	if node, ok = nodeManager.Nodes[name]; !ok {
		nodeManager.RWlock.RUnlock()
		if !nodeManager.stor.SupportWatcher() {
			defer conn.Close()
			plog.ErrorNode(name, "Could not find this node.")
			err = sendProtoMessage(conn, common.ErrNodeNotExist.Error(), ACTION_SESSION_DROP, clientTimeout)
			if err != nil {
				plog.ErrorNode(name, err)
			}
			return nil, common.ErrNodeNotExist
		}
		err = redirect(conn, name)
		if err != nil {
			conn.Close()
			return nil, err
		}
		return nil, nil
	}
	nodeManager.RWlock.RUnlock()
	return node, nil
}
