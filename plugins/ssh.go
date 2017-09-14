package plugins

import (
	"io/ioutil"
	"log"
	"time"

	"errors"
	"fmt"

	"github.com/chenglch/consoleserver/common"
	"github.com/chenglch/consoleserver/console"
	"golang.org/x/crypto/ssh"
)

var (
	plog *common.Logger
)

func init() {
	plog = common.GetLogger("github.com/chenglch/consoleserver/console/console")
}

type SSHConsole struct {
	node           string // session name
	user           string
	password       string
	privateKeyFile string
	host           string
	client         *ssh.Client
	session        *ssh.Session
}

func NewSSHConsole(host string, port string, user string, password string, pKeyFilepath string, node string) (*SSHConsole, error) {
	var sshInst *SSHConsole
	if pKeyFilepath != "" {
		sshInst = &SSHConsole{host: fmt.Sprintf("%s:%s", host, port), user: user, privateKeyFile: pKeyFilepath, node: node}
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

func (s *SSHConsole) startConsole() (*console.Console, error) {
	tty := console.Tty{}
	ttyWidth, err := tty.Width()
	if err != nil {
		return nil, err
	}
	ttyHeight, err := tty.Height()
	if err != nil {
		return nil, err
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
	console := console.NewConsole(sshIn, sshOut, s, s.node)
	return console, nil
}

func (s *SSHConsole) Start() (*console.Console, error) {
	err := s.connectToHost()
	if err != nil {
		plog.ErrorNode(s.node, err.Error())
		return nil, err
	}
	console, err := s.startConsole()
	if err != nil {
		plog.ErrorNode(s.node, err.Error())
		return nil, err
	}
	return console, nil
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
