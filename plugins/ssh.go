package plugins

import (
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/chenglch/goconserver/common"
	"golang.org/x/crypto/ssh"
	"net"
	"os"
)

const (
	DRIVER_SSH = "ssh"
)

func init() {
	DRIVER_INIT_MAP[DRIVER_SSH] = NewSSHConsole
	DRIVER_VALIDATE_MAP[DRIVER_SSH] = func(name string, params map[string]string) error {
		if _, ok := params["host"]; !ok {
			return common.NewErr(common.INVALID_PARAMETER, fmt.Sprintf("%s: Please specify the parameter host", name))
		}
		if _, ok := params["user"]; !ok {
			return common.NewErr(common.INVALID_PARAMETER, fmt.Sprintf("%s: Please specify the parameter user", name))
		}
		_, ok1 := params["password"]
		_, ok2 := params["private_key"]
		if !ok1 && !ok2 {
			return common.NewErr(common.INVALID_PARAMETER, fmt.Sprintf("%s: Please specify the parameter private_key or password", name))
		}
		return nil
	}
}

type SSHConsole struct {
	node           string // session name
	user           string
	password       string
	privateKeyFile string
	host           string
	client         *ssh.Client
	session        *ssh.Session
	exit           chan struct{}
}

func NewSSHConsole(node string, params map[string]string) (ConsolePlugin, error) {
	var password, privateKey, port string
	if _, ok := params["host"]; !ok {
		plog.ErrorNode(node, "host parameter is not defined")
		return nil, common.NewErr(common.INVALID_PARAMETER, fmt.Sprintf("%s: Please specify the parameter host", node))
	}
	host := params["host"]
	if _, ok := params["port"]; !ok {
		port = "22"
	} else {
		port = params["port"]
	}
	if _, ok := params["user"]; !ok {
		plog.ErrorNode(node, "user parameter is not defined")
		return nil, common.NewErr(common.INVALID_PARAMETER, fmt.Sprintf("%s: Please specify the parameter user", node))
	}
	user := params["user"]
	if _, ok := params["password"]; ok {
		password = params["password"]
	}
	if _, ok := params["private_key"]; ok {
		privateKey = params["private_key"]
	}
	if privateKey == "" && password == "" {
		plog.ErrorNode(node, "private_key and password, at least one of the parameter should be specified")
		return nil, common.NewErr(common.INVALID_PARAMETER, fmt.Sprintf("%s: Please specify the parameter password or private_key", node))
	}
	sshConsole := SSHConsole{
		host:           fmt.Sprintf("%s:%s", host, port),
		user:           user,
		privateKeyFile: privateKey,
		password:       password,
		node:           node,
		exit:           make(chan struct{}, 0)}
	return &sshConsole, nil
}

func (s *SSHConsole) appendPrivateKeyAuthMethod(autoMethods *[]ssh.AuthMethod) {
	if s.privateKeyFile != "" {
		key, err := ioutil.ReadFile(s.privateKeyFile)
		if err != nil {
			log.Printf("host:%s\tThe private key file %s can not be parsed, Error:%s", s.host, s.privateKeyFile, err)
			return
		}

		signer, err := ssh.ParsePrivateKey([]byte(key))
		if err != nil {
			log.Printf("host:%s\tThe private key file %s can not be parsed, Error:%s", s.host, s.privateKeyFile, err)
			return
		}
		*autoMethods = append(*autoMethods, ssh.PublicKeys(signer))
	}
}

func (s *SSHConsole) appendPasswordAuthMethod(autoMethods *[]ssh.AuthMethod) {
	if s.password != "" {
		*autoMethods = append(*autoMethods, ssh.Password(s.password))
	}
}

func (s *SSHConsole) keepSSHAlive(cl *ssh.Client, conn net.Conn) error {
	const keepAliveInterval = time.Minute
	t := time.NewTicker(keepAliveInterval)
	defer t.Stop()
	for {
		plog.DebugNode(s.node, "Keep alive goroutine for ssh connection started")
		deadline := time.Now().Add(keepAliveInterval).Add(15 * time.Second)
		err := conn.SetDeadline(deadline)
		if err != nil {
			plog.ErrorNode(s.node, "Failed to set deadline for ssh connection")
			return common.ErrSetDeadline
		}
		select {
		case <-t.C:
			_, _, err = cl.SendRequest("keepalive@golang.org", true, nil)
			if err != nil {
				plog.ErrorNode(s.node, "Faild to send keepalive request")
				return common.ErrSendKeepalive
			}
		case <-s.exit:
			plog.DebugNode(s.node, "Exit keepalive goroutine")
			return nil
		}
	}
}

func (s *SSHConsole) connectToHost() error {
	var err error
	timeout := 5 * time.Second
	autoMethods := make([]ssh.AuthMethod, 0)
	s.appendPrivateKeyAuthMethod(&autoMethods)
	s.appendPasswordAuthMethod(&autoMethods)
	sshConfig := &ssh.ClientConfig{
		User:            s.user,
		Auth:            autoMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	conn, err := net.DialTimeout("tcp", s.host, timeout)
	if err != nil {
		return err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, s.host, sshConfig)
	if err != nil {
		return err
	}
	s.client = ssh.NewClient(c, chans, reqs)
	go s.keepSSHAlive(s.client, conn)
	s.session, err = s.client.NewSession()
	if err != nil {
		common.SafeClose(s.exit)
		s.client.Close()
		s.client = nil
		return err
	}
	return nil
}

func (s *SSHConsole) startConsole() (*BaseSession, error) {
	tty := common.Tty{}
	ttyWidth, ttyHeight, err := tty.GetSize(os.Stdin)
	if err != nil {
		plog.DebugNode(s.node, "Could not get tty size, use 80,80 as default")
		ttyHeight = 80
		ttyWidth = 80
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // Disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	if err := s.session.RequestPty("xterm-256color", ttyWidth, ttyHeight, modes); err != nil {
		plog.ErrorNode(s.node, err.Error())
		return nil, err
	}
	sshIn, err := s.session.StdinPipe()
	if err != nil {
		plog.ErrorNode(s.node, err.Error())
		return nil, err
	}
	sshOut, err := s.session.StdoutPipe()
	if err != nil {
		plog.ErrorNode(s.node, err.Error())
		return nil, err
	}
	// Start remote shell
	if err := s.session.Shell(); err != nil {
		plog.ErrorNode(s.node, err.Error())
		return nil, err
	}
	return &BaseSession{In: sshIn, Out: sshOut, Session: s}, nil
}

func (s *SSHConsole) Start() (*BaseSession, error) {
	err := s.connectToHost()
	if err != nil {
		plog.ErrorNode(s.node, err.Error())
		return nil, err
	}
	baseSession, err := s.startConsole()
	if err != nil {
		plog.ErrorNode(s.node, err.Error())
		return nil, err
	}
	return baseSession, nil
}

func (s *SSHConsole) Close() error {
	if s.client != nil {
		common.SafeClose(s.exit)
		err := s.client.Close()
		s.client = nil
		return err
	}
	return nil
}

func (s *SSHConsole) Wait() error {
	if s.session != nil {
		return s.session.Wait()
	}
	return nil
}
