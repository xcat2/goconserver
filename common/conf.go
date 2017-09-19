package common

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

var (
	serverConfig *ServerConfig
)

type ServerConfig struct {
	Global struct {
		Host          string `yaml:"host"`
		SSLKeyFile    string `yaml:"ssl_key_file"`
		SSLCertFile   string `yaml:"ssl_cert_file"`
		SSLCACertFile string `yaml:"ssl_ca_cert_file"`
		LogFile       string `yaml:"logfile"`
	}
	API struct {
		Port        string `yaml:"port"`
		HttpTimeout int    `yaml:"http_timeout"`
	}
	Console struct {
		Port          string `yaml:"port"`
		DataDir       string `yaml:"datadir"`
		LogDir        string `yaml:"logdir"`
		SocketTimeout int    `yaml:"socket_timeout"`
	}
}

func InitServerConfig(confFile string) (*ServerConfig, error) {
	serverConfig.Global.Host = "0.0.0.0"
	serverConfig.Global.LogFile = ""
	serverConfig.API.Port = "8089"
	serverConfig.API.HttpTimeout = 10
	serverConfig.Console.Port = "12345"
	serverConfig.Console.DataDir = "/var/lib/consoleserver/"
	serverConfig.Console.LogDir = "/var/log/consoleserver/nodes/"
	serverConfig.Console.SocketTimeout = 10
	data, err := ioutil.ReadFile(confFile)
	if err != nil {
		return serverConfig, nil
	}
	err = yaml.Unmarshal(data, &serverConfig)
	if err != nil {
		return serverConfig, err
	}
	return serverConfig, nil
}

func GetServerConfig() *ServerConfig {
	return serverConfig
}
