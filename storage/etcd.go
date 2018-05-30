package storage

import (
	"encoding/json"
	"fmt"
	"github.com/coreos/etcd/clientv3"
	"github.com/xcat2/goconserver/common"
	"github.com/xcat2/goconserver/storage/etcd"
	"os"
	"strings"
	"time"
)

const (
	STORAGE_ETCD    = "etcd"
	LOCK_PREFIX     = "lock"
	NODE_PREFIX     = "node"
	ENDPOINT_PREFIX = "endpoint"
)

func init() {
	STORAGE_INIT_MAP[STORAGE_ETCD] = newEtcdStorage
}

func EtcdKeyJoin(elems ...string) string {
	paths := make([]string, 1, len(elems)+1)
	paths[0] = "/" + serverConfig.Etcd.Prefix
	paths = append(paths, elems...)
	for i, e := range paths {
		if e != "" {
			return strings.Join(paths[i:], "/")
		}
	}
	return ""
}

type EtcdStorage struct {
	*Storage
	client *etcd.EtcdClient
	host   string
	// vhost is for the business service, host is the real hostname
	vhost string
}

func newEtcdStorage() StorInterface {
	var err error
	var hostname string
	stor := new(Storage)
	stor.async = true
	stor.Nodes = make(map[string]*Node)
	etcdStor := new(EtcdStorage)
	etcdStor.Storage = stor
	hostname, err = os.Hostname()
	if err != nil {
		panic(err)
	}
	etcdStor.vhost = serverConfig.Etcd.Vhost
	if etcdStor.vhost == "" {
		etcdStor.vhost = hostname
	}
	etcdStor.host = serverConfig.Global.Host
	if etcdStor.host == "" || etcdStor.host == "0.0.0.0" {
		etcdStor.host = hostname
	}
	// As client is used to send keepalive request, the client would not be closed
	etcdStor.client, err = etcd.NewEtcdClient(&serverConfig.Etcd)
	if err != nil {
		panic(err)
	}
	ready := make(chan struct{})
	go etcdStor.keepalive(ready)
	<-ready
	return etcdStor
}

func (self *EtcdStorage) keepalive(ready chan<- struct{}) {
	config := NewEndpointConfig(serverConfig.API.Port, serverConfig.Etcd.RpcPort, serverConfig.Console.Port, self.host)
	b, err := config.ToByte()
	if err != nil {
		panic(err)
	}
	s := string(b)
	for {
		err := self.client.RegisterAndKeepalive(EtcdKeyJoin(LOCK_PREFIX, self.vhost), EtcdKeyJoin(ENDPOINT_PREFIX, self.vhost), s, ready)
		if err != nil {
			plog.Error("Failed to register service")
		}
		time.Sleep(time.Duration(serverConfig.Etcd.ServiceHeartbeat) * time.Second)
	}
}

func (self *EtcdStorage) GetEndpoint(vhost string) (*EndpointConfig, error) {
	b, err := self.client.Get(EtcdKeyJoin(ENDPOINT_PREFIX, vhost))
	if err != nil {
		return nil, err
	}
	config := new(EndpointConfig)
	err = json.Unmarshal(b, config)
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	return config, nil
}

func (self *EtcdStorage) GetVhosts() (map[string]*EndpointConfig, error) {
	m, err := self.client.List(EtcdKeyJoin(ENDPOINT_PREFIX))
	if err != nil {
		return nil, err
	}
	ret := make(map[string]*EndpointConfig)
	for k, v := range m {
		config := new(EndpointConfig)
		err = json.Unmarshal([]byte(v), config)
		if err != nil {
			plog.Error(err)
			return nil, err
		}
		temp := strings.Split(k, "/")
		vhost := temp[len(temp)-1]
		ret[vhost] = config
	}
	return ret, nil
}

func (self *EtcdStorage) ImportNodes() {
	kvs, err := self.client.List(EtcdKeyJoin(NODE_PREFIX, self.vhost))
	if err != nil {
		plog.Error(fmt.Sprintf("Unable to import node from storage, Error: %s", err))
		panic(err)
	}
	for _, v := range kvs {
		node, err := UnmarshalNode([]byte(v))
		if err != nil {
			continue
		}
		self.Storage.Nodes[node.Name] = node
	}
}

func (self *EtcdStorage) GetNodeCountEachHost() (map[string]int, error) {
	var err error
	hosts, err := self.GetVhosts()
	if err != nil {
		return nil, err
	}
	nodeCountMap := make(map[string]int)
	for host, _ := range hosts {
		nodeCountMap[host], err = self.client.Count(EtcdKeyJoin(NODE_PREFIX, host))
		if err != nil {
			return nil, err
		}
	}
	return nodeCountMap, nil
}

func (self *EtcdStorage) ListNodeWithHost() (map[string]string, error) {
	nodeToHost := make(map[string]string)
	resp, err := self.client.Keys(EtcdKeyJoin(NODE_PREFIX))
	if err != nil {
		return nil, err
	}
	for _, k := range resp {
		temp := strings.Split(string(k), "/")
		nodeToHost[temp[len(temp)-1]] = temp[len(temp)-2]
	}
	return nodeToHost, nil
}

func (self *EtcdStorage) NotifyPersist(record interface{}, action int) error {
	var err error
	switch action {
	case ACTION_PUT:
		err = self.putNode(record)
	case ACTION_MULTIPUT:
		err = self.putNodes(record)
	case ACTION_DEL:
		err = self.delNode(record)
	case ACTION_MULTIDEL:
		err = self.delNodes(record)
	}
	if err != nil {
		plog.Error(err)
	}
	return err
}

func (self *EtcdStorage) PersistWatcher(c chan<- interface{}) {
	key := EtcdKeyJoin(NODE_PREFIX, self.vhost)
	fc := func(events []*clientv3.Event, c chan<- interface{}) {
		for _, event := range events {
			switch event.Type {
			// put
			case 0:
				node, err := UnmarshalNode(event.Kv.Value)
				if err != nil {
					plog.Warn(err.Error())
					continue
				}
				c <- NewEventData(ACTION_PUT, node)
			// del
			case 1:
				temp := strings.Split(string(event.Kv.Key), "/")
				name := temp[len(temp)-1]
				c <- NewEventData(ACTION_DEL, name)
			}
		}
	}
	self.client.Watch(key, fc, c)
}

func (self *EtcdStorage) SupportWatcher() bool {
	return true
}

func (self *EtcdStorage) putNode(record interface{}) error {
	var ok bool
	var node *Node
	if node, ok = record.(*Node); !ok {
		return common.ErrInvalidType
	}
	dispatcher := newDispatcher(self)
	host, err := dispatcher.PeekPutHost(node)
	if err != nil {
		return nil
	}
	key := EtcdKeyJoin(NODE_PREFIX, host, node.Name)
	b, err := json.Marshal(*node)
	if err != nil {
		plog.ErrorNode(node.Name, err)
		return err
	}
	err = self.client.Put(key, b)
	if err != nil {
		return err
	}
	return nil
}

func (self *EtcdStorage) putNodes(record interface{}) error {
	var ok bool
	var nodes []Node
	if nodes, ok = record.([]Node); !ok {
		return common.ErrInvalidType
	}
	dispatcher := newDispatcher(self)
	m, err := dispatcher.PeekPutHostMap(nodes)
	if err != nil {
		return err
	}
	data := make(map[string]string)
	for node, host := range m {
		b, err := json.Marshal(node)
		if err != nil {
			plog.ErrorNode(node.Name, err)
			return err
		}
		key := EtcdKeyJoin(NODE_PREFIX, host, node.Name)
		data[key] = string(b)
	}
	err = self.client.MultiPut(data)
	if err != nil {
		return err
	}
	return nil
}

func (self *EtcdStorage) delNode(record interface{}) error {
	var ok bool
	var name string
	if name, ok = record.(string); !ok {
		return common.ErrInvalidType
	}
	dispatcher := newDispatcher(self)
	host, err := dispatcher.PeekDelHost(name)
	if err != nil {
		return err
	}
	key := EtcdKeyJoin(NODE_PREFIX, host, name)
	err = self.client.Del(key)
	if err != nil {
		return err
	}
	return err
}

func (self *EtcdStorage) delNodes(record interface{}) error {
	var ok bool
	var names []string
	if names, ok = record.([]string); !ok {
		return common.ErrInvalidType
	}
	dispatcher := newDispatcher(self)
	hostMap, err := dispatcher.PeekDelHostMap(names)
	if err != nil {
		return err
	}
	keys := make([]string, len(hostMap))
	i := 0
	for name, host := range hostMap {
		keys[i] = EtcdKeyJoin(NODE_PREFIX, host, name)
		i++
	}
	self.client.MultiDel(keys)
	if err != nil {
		return err
	}
	return nil
}
