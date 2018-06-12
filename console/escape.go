package console

import (
	"fmt"
	"github.com/xcat2/goconserver/common"
	"io"
	"net"
	"sync"
	"time"
)

const (
	ESCAPE_CTRL_E     = '\x05'
	ESCAPE_C          = 'c'
	CLIENT_CMD_EXIT   = '.'
	CLIENT_CMD_HELP   = '?'
	CLIENT_CMD_REPLAY = 'r'
	CLIENT_CMD_WHO    = 'w'
	CLIENT_CMD_LOCAL  = 'l'

	SEARCH_BUF_SIZE = 8
)

var (
	EXIT_SEQUENCE = [...]byte{ESCAPE_CTRL_E, ESCAPE_C, '.'} // ctrl-e, c
	clientEscape  *EscapeClientSystem
	serverEscape  *EscapeServerSystem
)

type EscapeClientHandler func(net.Conn, interface{}, string, byte) error
type EscapeServerHandler func(io.Writer, byte) error

func serverBreakSequenceHandler(writer io.Writer, last byte) error {
	var err error
	// slice start from 0, but the escape key start from 1
	sequence := serverEscape.sequences[last-'0'-1]
	for _, ch := range sequence.seqs {
		err = common.SafeWrite(writer, []byte{byte(ch)})
		if err != nil {
			plog.Error(err)
			return err
		}
		time.Sleep(time.Duration(sequence.delay) * time.Microsecond)
	}
	return nil
}

func clientBreakSequenceHandler(conn net.Conn, c interface{}, node string, last byte) error {
	printBreakSequence(last)
	if conn == nil {
		return nil
	}
	if last == '?' {
		congo := NewCongoClient(clientConfig.HTTPUrl)
		ret, err := congo.listBreakSequence()
		if err != nil {
			printFatalErr(err)
			return err
		}
		for _, s := range ret {
			fmt.Printf("%s\r\n", s)
		}
		return nil
	}
	err := common.Network.SendByteWithLength(conn.(net.Conn), []byte{ESCAPE_CTRL_E, ESCAPE_C, CLIENT_CMD_LOCAL, last})
	if err != nil {
		return err
	}
	return nil
}

func clientEscapeExitHandler(conn net.Conn, c interface{}, node string, last byte) error {
	var err error
	client := c.(*ConsoleClient)
	client.close()
	if conn != nil {
		err = common.Network.SendByteWithLength(conn.(net.Conn), EXIT_SEQUENCE[0:])
		if err != nil {
			return err
		}
	}
	client.retry = false
	printConsoleDisconnectPrompt(client.mode, node)
	return nil
}

func clientEscapeHelpHandler(conn net.Conn, c interface{}, node string, last byte) error {
	printConsoleHelpMsg()
	return nil
}

func clientEscapeReplayHandler(conn net.Conn, c interface{}, node string, last byte) error {
	congo := NewCongoClient(clientConfig.HTTPUrl)
	ret, err := congo.replay(node)
	if err != nil {
		if ret != "" {
			printConsoleCmdErr(ret)
		} else {
			printConsoleCmdErr(err)
		}
		return err
	}
	printConsoleReplay(ret)
	return nil
}

func clientEscapeWhoHandler(conn net.Conn, c interface{}, node string, last byte) error {
	congo := NewCongoClient(clientConfig.HTTPUrl)
	ret, err := congo.listUser(node)
	if err != nil {
		if ret != nil {
			printConsoleCmdErr(ret)
		} else {
			printConsoleCmdErr(err)
		}
		return err
	}
	printCRLF()
	for _, v := range ret["users"].([]interface{}) {
		printConsoleUser(v.(string))
	}
	return nil
}

type EscapeSearcher struct {
	node *EscapeNode
	len  int
	buf  [SEARCH_BUF_SIZE]byte
}

func NewEscapeSearcher(root *EscapeNode) *EscapeSearcher {
	return &EscapeSearcher{node: root, len: 0}
}

type EscapeNode struct {
	next     map[byte]*EscapeNode
	refCount int
	handler  interface{}
}

func NewEscapeNode() *EscapeNode {
	return &EscapeNode{refCount: 1, handler: nil, next: make(map[byte]*EscapeNode)}
}

type BreakSequence struct {
	seqs  string
	delay int // ms
}

func NewBreakSequence(seqs string, delay int) *BreakSequence {
	return &BreakSequence{
		seqs:  seqs,
		delay: delay,
	}
}

type EscapeSystem struct {
	rwLock *sync.RWMutex
	root   *EscapeNode
}

func (self *EscapeSystem) Register(s []byte, handler interface{}) {
	if len(s) == 0 {
		return
	}
	var ok bool
	var node *EscapeNode
	self.root.refCount++
	entry := self.root
	self.rwLock.Lock()
	defer self.rwLock.Unlock()
	for _, ch := range s {
		if node, ok = entry.next[ch]; !ok {
			node = NewEscapeNode()
			entry.next[ch] = node
		} else {
			node.refCount++
		}
		entry = node
	}
	node.handler = handler
}

func (self *EscapeSystem) exist(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	var ok bool
	entry := self.root
	self.rwLock.RLock()
	defer self.rwLock.RUnlock()
	for _, ch := range s {
		if entry, ok = entry.next[ch]; !ok {
			return false
		}
	}
	return true
}

func (self *EscapeSystem) Unregister(s []byte) error {
	if !self.exist(s) {
		return common.ErrNotExist
	}
	self.root.refCount--
	var ok bool
	var node *EscapeNode
	self.rwLock.Lock()
	defer self.rwLock.Unlock()
	entry := self.root
	for _, ch := range s {
		if node, ok = entry.next[ch]; !ok {
			return common.ErrNotExist
		}
		entry.refCount--
		delete(entry.next, ch)
		entry = node
	}
	return nil
}

type EscapeClientSystem struct {
	*EscapeSystem
}

func NewEscapeClientSystem() *EscapeClientSystem {
	escapeSystem := &EscapeSystem{
		root:   NewEscapeNode(),
		rwLock: new(sync.RWMutex),
	}
	escapeSystem.Register([]byte{ESCAPE_CTRL_E, ESCAPE_C, CLIENT_CMD_EXIT}, clientEscapeExitHandler)
	escapeSystem.Register([]byte{ESCAPE_CTRL_E, ESCAPE_C, CLIENT_CMD_HELP}, clientEscapeHelpHandler)
	escapeSystem.Register([]byte{ESCAPE_CTRL_E, ESCAPE_C, CLIENT_CMD_REPLAY}, clientEscapeReplayHandler)
	escapeSystem.Register([]byte{ESCAPE_CTRL_E, ESCAPE_C, CLIENT_CMD_WHO}, clientEscapeWhoHandler)
	for i := 1; i <= 9; i++ {
		escapeSystem.Register([]byte{ESCAPE_CTRL_E, ESCAPE_C, CLIENT_CMD_LOCAL, byte(i) + '0'}, clientBreakSequenceHandler)
	}
	escapeSystem.Register([]byte{ESCAPE_CTRL_E, ESCAPE_C, CLIENT_CMD_LOCAL, '?'}, clientBreakSequenceHandler)
	return &EscapeClientSystem{escapeSystem}
}

// Search and cache the charactor
// return buffered, if return true, this character has been buffered in searcher
func (self *EscapeClientSystem) Search(conn net.Conn, b byte, searcher *EscapeSearcher) (bool, EscapeClientHandler, error) {
	var ok bool
	self.rwLock.RLock()
	defer self.rwLock.RUnlock()
	if searcher.node, ok = searcher.node.next[b]; !ok {
		searcher.node = self.root
		if conn != nil && searcher.len != 0 {
			// if the character is not found, send the buffer to the remote and clear the buffer
			err := common.Network.SendByteWithLength(conn, searcher.buf[:searcher.len])
			if err != nil {
				// do not has buffer to send
				return false, nil, err
			}
		}
		searcher.len = 0
		return false, nil, nil
	}
	if searcher.len == SEARCH_BUF_SIZE {
		return false, nil, common.ErrOutOfQuota
	}
	searcher.buf[searcher.len] = b
	searcher.len++
	if searcher.node.handler != nil {
		searcher.len = 0
		// match the pattern, return handler, no buffer to send
		return false, searcher.node.handler.(func(net.Conn, interface{}, string, byte) error), nil
	}
	// this character has been add to the search buffer
	return true, nil, nil
}

type EscapeServerSystem struct {
	*EscapeSystem
	sequences []*BreakSequence
}

func NewEscapeServerSystem() *EscapeServerSystem {
	escapeSystem := &EscapeSystem{
		root:   NewEscapeNode(),
		rwLock: new(sync.RWMutex),
	}
	breakSequences := make([]*BreakSequence, 9)
	i := 1
	for _, sequence := range serverConfig.Console.BreakSequences {
		escapeSystem.Register([]byte{ESCAPE_CTRL_E, ESCAPE_C, CLIENT_CMD_LOCAL, byte(i) + '0'}, serverBreakSequenceHandler)
		breakSequences[i-1] = NewBreakSequence(sequence.Sequence, sequence.Delay)
		i++
	}
	for i <= 9 {
		escapeSystem.Register([]byte{ESCAPE_CTRL_E, ESCAPE_C, CLIENT_CMD_LOCAL, byte(i) + '0'}, serverBreakSequenceHandler)
		breakSequences[i-1] = NewBreakSequence("~B", 250)
		i++
	}
	return &EscapeServerSystem{escapeSystem, breakSequences}
}

func (self *EscapeServerSystem) GetSequences() []string {
	ret := make([]string, len(self.sequences))
	for i := 0; i < len(self.sequences); i++ {
		ret[i] = fmt.Sprintf("key: %d string: %s, delay: %d", i+1, self.sequences[i].seqs, self.sequences[i].delay)
	}
	return ret
}

// return buffered, if return true, this character has been buffered in searcher
func (self *EscapeServerSystem) Search(writer io.Writer, b byte, searcher *EscapeSearcher) (bool, EscapeServerHandler, error) {
	var ok bool
	var err error
	self.rwLock.RLock()
	defer self.rwLock.RUnlock()
	if searcher.node, ok = searcher.node.next[b]; !ok {
		searcher.node = self.root
		// if the character is not found, send the buffer to the remote and clear the buffer
		if searcher.len > 0 {
			err = common.SafeWrite(writer, searcher.buf[:searcher.len])
			if err != nil {
				plog.Error(err)
				return false, nil, err
			}
		}
		searcher.len = 0
		return false, nil, nil
	}
	if searcher.len == SEARCH_BUF_SIZE {
		return false, nil, common.ErrOutOfQuota
	}
	searcher.buf[searcher.len] = b
	searcher.len++
	if searcher.node.handler != nil {
		searcher.len = 0
		// match the pattern, return handler, no buffer to send
		return false, searcher.node.handler.(func(io.Writer, byte) error), nil
	}
	// this character has been add to the search buffer
	return true, nil, nil
}

func GetServerEscape() *EscapeServerSystem {
	return serverEscape
}
