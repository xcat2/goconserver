package plugins

import (
	"io/ioutil"
	"log"
	"time"

	"errors"
	"fmt"

	"github.com/chenglch/consoleserver/common"
	"golang.org/x/crypto/ssh"
	"os"
)

type SSHConsole struct {
	node           string // session name
	user           string
	password       string
	privateKeyFile string
	host           string
	client         *ssh.Client
	session        *ssh.Session
}

func NewSSHConsole(node string, params map[string]string) (*SSHConsole, error) {
	var password, privateKey, port string
	if _, ok := params["host"]; !ok {
		plog.ErrorNode(node, "host parameter is not defined")
		return nil, errors.New("host parameter is not defined")
	}
	host := params["host"]
	if _, ok := params["port"]; !ok {
		port = "22"
	} else {
		port = params["port"]
	}
	if _, ok := params["user"]; !ok {
		plog.ErrorNode(node, "user parameter is not defined")
		return nil, errors.New("user parameter is not defined")
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
		return nil, errors.New("private_key and password, at least one of the parameter should be specified")
	}

	var sshInst *SSHConsole
	if privateKey != "" {
		sshInst = &SSHConsole{host: fmt.Sprintf("%s:%s", host, port), user: user, privateKeyFile: privateKey, node: node}
	} else if password != "" {
		sshInst = &SSHConsole{host: fmt.Sprintf("%s:%s", host, port), user: user, password: password, node: node}
	} else {
		return nil, errors.New("Please specify the password or private key")
	}
	return sshInst, nil
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

func (s *SSHConsole) connectToHost() error {
	var err error
	autoMethods := make([]ssh.AuthMethod, 0)
	s.appendPrivateKeyAuthMethod(&autoMethods)
	s.appendPasswordAuthMethod(&autoMethods)
	sshConfig := &ssh.ClientConfig{
		User:            s.user,
		Auth:            autoMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	s.client, err = ssh.Dial("tcp", s.host, sshConfig)
	if err != nil {
		return err
	}

	s.session, err = s.client.NewSession()
	if err != nil {
		s.client.Close()
		return err
	}
	return nil
}

func (s *SSHConsole) startConsole() (*BaseSession, error) {
	tty := common.Tty{}
	ttyWidth, ttyHeight, err := tty.GetSize(os.Stdin)
	if err != nil {
		plog.WarningNode(s.node, "Could not get tty size, use 80,80 as default")
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
		return s.client.Close()
	}
	return nil
}

func (s *SSHConsole) Wait() error {
	if s.session != nil {
		return s.session.Wait()
	}
	return nil
}
