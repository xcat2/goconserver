package storage

import (
	"encoding/json"
	"fmt"
	"github.com/xcat2/goconserver/common"
)

const (
	ACTION_MULTIDEL = 4
	ACTION_DEL      = 3
	ACTION_MULTIPUT = 2
	ACTION_PUT      = 1
	ACTION_NIL      = -1
)

var (
	serverConfig     = common.GetServerConfig()
	plog             = common.GetLogger("github.com/xcat2/goconserver/storage")
	STORAGE_INIT_MAP = map[string]func() StorInterface{}
)

type EventData struct {
	Action int
	Data   interface{}
}

func NewEventData(action int, data interface{}) *EventData {
	return &EventData{Action: action, Data: data}
}

type EndpointConfig struct {
	ApiPort     string `json:"api_port"`
	RpcPort     string `json:"rpc_port"`
	ConsolePort string `json:"console_port`
	Host        string `json:"host"`
}

func NewEndpointConfig(apiPort, rpcPort, consolePort, host string) *EndpointConfig {
	return &EndpointConfig{ApiPort: apiPort, RpcPort: rpcPort, ConsolePort: consolePort, Host: host}
}

func (self *EndpointConfig) ToByte() ([]byte, error) {
	b, err := json.Marshal(self)
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	return b, nil
}

type Node struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver"` // node type cmd, ssh, ipmitool
	Params   map[string]string `json:"params"`
	Ondemand bool              `json:"ondemand, true"`
	Labels   map[string]string `json:labels, omitempty`
}

func NewNode() *Node {
	return new(Node)
}

func UnmarshalNode(b []byte) (*Node, error) {
	node := new(Node)
	if err := json.Unmarshal(b, node); err != nil {
		plog.Error(err)
		return nil, err
	}
	if node.Name == "" {
		plog.Error("Node name is not defined")
		return nil, common.ErrNodeNotExist
	}
	if node.Driver == "" {
		plog.ErrorNode(node.Name, common.ErrDriverNotExist)
		return nil, common.ErrDriverNotExist
	}
	return node, nil
}

type Storage struct {
	Nodes map[string]*Node
	async bool
}

func (s *Storage) GetNodes() map[string]*Node {
	return s.Nodes
}

type StorInterface interface {
	ImportNodes() // import nodes to Storage.Nodes
	PersistWatcher(eventChan chan<- interface{})
	GetVhosts() (map[string]*EndpointConfig, error)
	GetNodeCountEachHost() (map[string]int, error) // key host, value int count
	GetNodes() map[string]*Node
	GetEndpoint(host string) (*EndpointConfig, error)
	NotifyPersist(interface{}, int) error
	SupportWatcher() bool
	ListNodeWithHost() (map[string]string, error) // key node name, value host
}

func NewStorage(storType string) (StorInterface, error) {
	if _, ok := STORAGE_INIT_MAP[storType]; !ok {
		return nil, common.NewErr(common.INVALID_PARAMETER, fmt.Sprintf("The storage type %s is not exist", storType))
	}
	return STORAGE_INIT_MAP[storType](), nil
}
