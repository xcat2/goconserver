package common

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"strconv"
)

var (
	serverConfig *ServerConfig
	clientConfig *ClientConfig
	CONF_FILE    string
)

type ServerConfig struct {
	Global struct {
		Host          string `yaml:"host"`
		SSLKeyFile    string `yaml:"ssl_key_file"`
		SSLCertFile   string `yaml:"ssl_cert_file"`
		SSLCACertFile string `yaml:"ssl_ca_cert_file"`
		LogFile       string `yaml:"logfile"`
		LogLevel      string `yaml:"log_level"` // debug, info, warn, error, fatal, panic
		Worker        int    `yaml:"worker"`
		StorageType   string `yaml:"storage_type"`
	}
	API struct {
		Port        string `yaml:"port"`
		HttpTimeout int    `yaml:"http_timeout"`
	}
	Console struct {
		Port              string `yaml:"port"`
		DataDir           string `yaml:"datadir"`
		LogDir            string `yaml:"logdir"`
		LogTimestamp      bool   `yaml:"log_timestamp"`
		ReplayLines       int    `yaml:"replay_lines"`
		ClientTimeout     int    `yaml:"client_timeout"`
		TargetTimeout     int    `yaml:"target_timeout"`
		ReconnectInterval int    `yaml:"reconnect_interval"`
		RPCPort           string `yaml:"rpcport"`
	}
	Etcd struct {
		DailTimeout     int    `yaml:"dail_timeout"`
		RequestTimeout  int    `yaml:"request_timeout"`
		Endpoints       string `yaml:"endpoints"`
		ServerHeartbeat int64  `yaml:"server_heartbeat"`
	}
}

func InitServerConfig(confFile string) (*ServerConfig, error) {
	serverConfig.Global.Host = "127.0.0.1"
	serverConfig.Global.LogFile = ""
	serverConfig.Global.Worker = 1
	serverConfig.Global.LogLevel = "info"
	serverConfig.Global.StorageType = "file"
	serverConfig.API.Port = "12429"
	serverConfig.API.HttpTimeout = 10
	serverConfig.Console.Port = "12430"
	serverConfig.Console.DataDir = "/var/lib/goconserver/"
	serverConfig.Console.LogDir = "/var/log/goconserver/nodes/"
	serverConfig.Console.LogTimestamp = true
	serverConfig.Console.ReplayLines = 30
	serverConfig.Console.ClientTimeout = 30
	serverConfig.Console.TargetTimeout = 30
	serverConfig.Console.ReconnectInterval = 5
	serverConfig.Console.RPCPort = "12431" // only for async storage type
	serverConfig.Etcd.DailTimeout = 5
	serverConfig.Etcd.RequestTimeout = 2
	serverConfig.Etcd.Endpoints = "127.0.0.1:2379"
	serverConfig.Etcd.ServerHeartbeat = 5
	data, err := ioutil.ReadFile(confFile)
	if err != nil {
		return serverConfig, nil
	}
	err = yaml.Unmarshal(data, &serverConfig)
	if err != nil {
		return serverConfig, err
	}
	CONF_FILE = confFile
	return serverConfig, nil
}

func GetServerConfig() *ServerConfig {
	return serverConfig
}

type ClientConfig struct {
	SSLKeyFile     string
	SSLCertFile    string
	SSLCACertFile  string
	HTTPUrl        string
	ConsolePort    string
	ConsoleTimeout int
	ServerHost     string
}

func NewClientConfig() (*ClientConfig, error) {
	var err error
	clientConfig = new(ClientConfig)
	clientConfig.HTTPUrl = "http://127.0.0.1:12429"
	clientConfig.ServerHost = "127.0.0.1"
	clientConfig.ConsolePort = "12430"
	clientConfig.ConsoleTimeout = 30
	if os.Getenv("CONGO_URL") != "" {
		clientConfig.HTTPUrl = os.Getenv("CONGO_URL")
	}
	if os.Getenv("CONGO_SERVER_HOST") != "" {
		clientConfig.ServerHost = os.Getenv("CONGO_SERVER_HOST")
	}
	if os.Getenv("CONGO_PORT") != "" {
		clientConfig.ConsolePort = os.Getenv("CONGO_PORT")
	}
	if os.Getenv("CONGO_CONSOLE_TIMEOUT") != "" {
		clientConfig.ConsoleTimeout, err = strconv.Atoi(os.Getenv("CONGO_CONSOLE_TIMEOUT"))
		if err != nil {
			return nil, err
		}
	}
	if os.Getenv("CONGO_SSL_KEY") != "" {
		clientConfig.SSLKeyFile = os.Getenv("CONGO_SSL_KEY")
	}
	if os.Getenv("CONGO_SSL_CERT") != "" {
		clientConfig.SSLCertFile = os.Getenv("CONGO_SSL_CERT")
	}
	if os.Getenv("CONGO_SSL_CA_CERT") != "" {
		clientConfig.SSLCACertFile = os.Getenv("CONGO_SSL_CA_CERT")
	}
	return clientConfig, nil
}

func GetClientConfig() *ClientConfig {
	return clientConfig
}
