package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/coreos/etcd/clientv3"
	"github.com/xcat2/goconserver/common"
	"os"
	"reflect"
	"strings"
	"time"
)

const (
	STORAGE_ETCD = "etcd"
)

func init() {
	STORAGE_INIT_MAP[STORAGE_ETCD] = newEtcdStorage
}

type EtcdStorage struct {
	*Storage
	endpoints      []string
	dialTimeout    time.Duration
	requestTimeout time.Duration
	host           string
}

func newEtcdStorage() StorInterface {
	var err error
	stor := new(Storage)
	stor.async = true
	stor.Nodes = make(map[string]*Node)
	etcdStor := new(EtcdStorage)
	etcdStor.Storage = stor
	etcdStor.endpoints = strings.Split(serverConfig.Etcd.Endpoints, " ")
	etcdStor.dialTimeout = time.Duration(serverConfig.Etcd.DailTimeout) * time.Second
	etcdStor.requestTimeout = time.Duration(serverConfig.Etcd.RequestTimeout) * time.Second
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	etcdStor.host = hostname
	err = etcdStor.register()
	if err != nil {
		panic(err)
	}
	return etcdStor
}

func (self *EtcdStorage) register() error {
	cli, err := self.getClient()
	if err != nil {
		return err
	}
	defer cli.Close()
	lease := clientv3.NewLease(cli)
	resp, err := lease.Grant(context.TODO(), serverConfig.Etcd.ServerHeartbeat)
	if err != nil {
		plog.Error(fmt.Sprintf("Could not request lease from etcd, error: %s", err))
		return err
	}
	key := fmt.Sprintf("/goconserver/hosts/%s", self.host)
	_, err = cli.Put(context.TODO(), key, self.host, clientv3.WithLease(resp.ID))
	if err != nil {
		plog.Error(fmt.Sprintf("Could not update host on etcd, error: %s", err))
		return err
	}
	go func() {
		t := serverConfig.Etcd.ServerHeartbeat - 2
		if t <= 0 {
			t = 1
		}
		cli, err := self.getClient()
		if err != nil {
			plog.Error(err)
		}
		defer cli.Close()
		for {
			time.Sleep(time.Duration(t) * time.Second)
			_, err = cli.KeepAliveOnce(context.TODO(), resp.ID)
			if err != nil {
				plog.Error(fmt.Sprintf("Could not update host on etcd, error: %s", err))
			}
		}
	}()
	return nil
}

func (self *EtcdStorage) getHosts(cli *clientv3.Client) ([]string, error) {
	var err error
	if cli == nil {
		return nil, common.ErrETCDNotInit
	}
	resp, err := cli.Get(context.TODO(), "/goconserver/hosts", clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	hosts := make([]string, 0, len(resp.Kvs))
	for _, v := range resp.Kvs {
		hosts = append(hosts, string(v.Value))
	}
	return hosts, nil
}

func (self *EtcdStorage) getClient() (*clientv3.Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   self.endpoints,
		DialTimeout: self.dialTimeout,
	})
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func (self *EtcdStorage) ImportNodes() {
	dirKey := fmt.Sprintf("/goconserver/%s/nodes", self.host)
	cli, err := self.getClient()
	if err != nil {
		plog.Error(err)
		return
	}
	defer cli.Close()
	resp, err := cli.Get(context.TODO(), dirKey, clientv3.WithPrefix())
	if err != nil {
		plog.Error(err)
		return
	}
	for _, v := range resp.Kvs {
		node := NewNode()
		val := v.Value
		if err := json.Unmarshal(val, node); err != nil {
			plog.Error(err)
			continue
		}
		if node.Name == "" {
			plog.Error("Skip this record as node name is not defined")
			continue
		}
		if node.Driver == "" {
			plog.ErrorNode(node.Name, "Driver is not defined")
			continue
		}
		self.Storage.Nodes[node.Name] = node
	}
}

func (self *EtcdStorage) getHostCount(cli *clientv3.Client) (map[string]int, error) {
	var err error
	if cli == nil {
		return nil, common.ErrETCDNotInit
	}
	hosts, err := self.getHosts(cli)
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	hostsCount := make(map[string]int)
	for _, host := range hosts {
		key := fmt.Sprintf("/goconserver/%s/nodes/", host)
		resp, err := cli.Get(context.TODO(), key, clientv3.WithPrefix(), clientv3.WithCountOnly())
		if err != nil {
			plog.Error(err)
			hostsCount[host] = -1
			continue
		}
		hostsCount[host] = int(resp.Count)
	}
	return hostsCount, nil
}

func (self *EtcdStorage) getMinHost(hostsCount map[string]int) string {
	minHost := ""
	minCount := common.Maxint32
	for k, v := range hostsCount {
		if v == -1 {
			continue
		}
		minCount = common.If(v < minCount, v, minCount).(int)
		if v == minCount {
			minHost = k
		}
	}
	return minHost
}

func (self *EtcdStorage) GetHosts() []string {
	cli, err := self.getClient()
	if err != nil {
		plog.Error(err)
		return nil
	}
	defer cli.Close()
	hosts, err := self.getHosts(cli)
	if err != nil {
		plog.Error(err)
		return nil
	}
	return hosts
}

func (self *EtcdStorage) ListNodeWithHost() map[string]string {
	cli, err := self.getClient()
	if err != nil {
		plog.Error(err)
		return nil
	}
	defer cli.Close()
	hosts, err := self.getHosts(cli)
	if err != nil {
		plog.Error(err)
		return nil
	}
	hostNodes := make(map[string]string)
	for _, host := range hosts {
		key := fmt.Sprintf("/goconserver/%s/nodes/", host)
		resp, err := cli.Get(context.TODO(), key, clientv3.WithPrefix())
		if err != nil {
			plog.Error(err)
			continue
		}
		for _, v := range resp.Kvs {
			if string(v.Key) == key {
				continue
			}
			vals := strings.Split(string(v.Key), "/")
			hostNodes[vals[len(vals)-1]] = host
		}
	}
	return hostNodes
}

func (self *EtcdStorage) NotifyPersist(nodes interface{}, action int) {
	if action == common.ACTION_PUT {
		if reflect.TypeOf(nodes).Kind() != reflect.Map {
			plog.Error("The persistance format is not Map, ignore.")
		}
		cli, err := self.getClient()
		if err != nil {
			plog.Error(err)
			return
		}
		defer cli.Close()
		hostsCount, err := self.getHostCount(cli)
		if err != nil {
			plog.Error(err)
			return
		}
		for _, v := range nodes.(map[string][]Node)["nodes"] {
			host := self.getMinHost(hostsCount)
			if host == "" {
				plog.Error("Could not find proper host")
				return
			}
			key := fmt.Sprintf("/goconserver/%s/nodes/%s", host, v.Name)
			b, err := json.Marshal(v)
			if err != nil {
				plog.ErrorNode(v.Name, err)
				continue
			}
			_, err = cli.Put(context.TODO(), key, string(b))
			if err != nil {
				plog.ErrorNode(v.Name, err)
				continue
			}
			hostsCount[host]++
		}
	} else if action == common.ACTION_DELETE {
		if reflect.TypeOf(nodes).Kind() != reflect.Slice {
			plog.Error("The persistance format is not Slice, ignore.")
		}
		cli, err := self.getClient()
		if err != nil {
			plog.Error(err)
			return
		}
		defer cli.Close()
		for _, v := range nodes.([]string) {
			key := fmt.Sprintf("/goconserver/%s/nodes/%s", self.host, v)
			_, err = cli.Delete(context.TODO(), key)
			if err != nil {
				plog.ErrorNode(v, err)
				continue
			}
		}
	} else {
		plog.Error("Not support")
	}
}

func (self *EtcdStorage) PersistWatcher(eventChan chan map[int][]byte) {
	cli, err := self.getClient()
	if err != nil {
		plog.Error(err)
		return
	}
	defer cli.Close()
	key := fmt.Sprintf("/goconserver/%s/nodes/", self.host)
	changes := cli.Watch(context.Background(), key, clientv3.WithPrefix())
	for resp := range changes {
		for _, ev := range resp.Events {
			if string(ev.Kv.Key) == key {
				continue
			}
			eventMap := make(map[int][]byte)
			if ev.Type == common.ACTION_PUT {
				eventMap[int(ev.Type)] = ev.Kv.Value
			} else if ev.Type == common.ACTION_DELETE {
				keys := strings.Split(string(ev.Kv.Key), "/")
				eventMap[int(ev.Type)] = []byte(keys[len(keys)-1])
			}
			eventChan <- eventMap
		}
	}
}

func (self *EtcdStorage) SupportWatcher() bool {
	return true
}
