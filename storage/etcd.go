package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chenglch/consoleserver/common"
	"github.com/coreos/etcd/clientv3"
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
	hostname       string
}

func newEtcdStorage() StorInterface {
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
	etcdStor.hostname = hostname
	err = etcdStor.register()
	if err != nil {
		panic(err)
	}
	return etcdStor
}

func (s *EtcdStorage) register() error {
	cli, err := s.getClient()
	if err != nil {
		return err
	}
	key := fmt.Sprintf("/consoleserver/hosts/%s", s.hostname)
	_, err = cli.Put(context.TODO(), key, s.hostname)
	if err != nil {
		plog.Error(err)
		return err
	}
	defer cli.Close()
	return nil
}

func (s *EtcdStorage) getHosts(cli *clientv3.Client) ([]string, error) {
	var err error
	if cli == nil {
		cli, err = s.getClient()
	}
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("/consoleserver/hosts/%s", s.hostname)
	resp, err := cli.Get(context.TODO(), key, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	hosts := make([]string, 0)
	for _, v := range resp.Kvs {
		hosts = append(hosts, string(v.Value))
	}
	return hosts, nil
}

func (s *EtcdStorage) getClient() (*clientv3.Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   s.endpoints,
		DialTimeout: s.dialTimeout,
	})
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func (s *EtcdStorage) ImportNodes() {
	dirKey := fmt.Sprintf("/consoleserver/%s/nodes", s.hostname)
	cli, err := s.getClient()
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
			plog.Error(errors.New("Skip this record as node name is not defined"))
			continue
		}
		if node.Driver == "" {
			plog.Error(errors.New("Driver is not defined"))
			continue
		}
		s.Storage.Nodes[node.Name] = node
	}
}

func (s *EtcdStorage) getHostCount(cli *clientv3.Client) map[string]int {
	var err error
	if cli == nil {
		cli, err = s.getClient()
		if err != nil {
			plog.Error(err)
			return nil
		}
	}
	hosts, err := s.getHosts(cli)
	if err != nil {
		plog.Error(err)
		return nil
	}
	hostsCount := make(map[string]int)
	for _, host := range hosts {
		key := fmt.Sprintf("/consoleserver/%s/nodes/", host)
		resp, err := cli.Get(context.TODO(), key, clientv3.WithPrefix(), clientv3.WithCountOnly())
		if err != nil {
			plog.Error(err)
			hostsCount[host] = -1
			continue
		}
		hostsCount[host] = int(resp.Count)
	}
	return hostsCount
}

func (s *EtcdStorage) getMinHost(hostsCount map[string]int) string {
	minHost := ""
	minCount := common.Maxint32
	for k, v := range hostsCount {
		minCount = common.If(v < minCount, v, minCount).(int)
		minHost = k
	}
	return minHost
}

func (s *EtcdStorage) NotifyPersist(nodes interface{}, action int) {
	if action == common.ACTION_PUT {
		if reflect.TypeOf(nodes).Kind() != reflect.Map {
			err := errors.New("The persistance format is not Map, ignore.")
			plog.Error(err)
		}
		cli, err := s.getClient()
		if err != nil {
			plog.Error(err)
			return
		}
		defer cli.Close()
		hostsCount := s.getHostCount(cli)
		for _, v := range nodes.(map[string][]Node)["nodes"] {
			hostname := s.getMinHost(hostsCount)
			key := fmt.Sprintf("/consoleserver/%s/nodes/%s", hostname, v.Name)
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
		}
	} else if action == common.ACTION_DELETE {
		if reflect.TypeOf(nodes).Kind() != reflect.Slice {
			err := errors.New("The persistance format is not Slice, ignore.")
			plog.Error(err)
		}
		cli, err := s.getClient()
		if err != nil {
			plog.Error(err)
			return
		}
		defer cli.Close()
		for _, v := range nodes.([]string) {
			key := fmt.Sprintf("/consoleserver/%s/nodes/%s", s.hostname, v)
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

func (s *EtcdStorage) PersistWatcher(eventChan chan map[int][]byte) {
	cli, err := s.getClient()
	if err != nil {
		plog.Error(err)
		return
	}
	defer cli.Close()
	key := fmt.Sprintf("/consoleserver/%s/nodes/", s.hostname)
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

func (s *EtcdStorage) IsAsync() bool {
	return true
}
