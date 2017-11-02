package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chenglch/goconserver/common"
	"github.com/coreos/etcd/clientv3"
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
	etcdStor.host = serverConfig.Global.Host
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
	defer cli.Close()
	lease := clientv3.NewLease(cli)
	resp, err := lease.Grant(context.TODO(), serverConfig.Etcd.ServerHeartbeat)
	if err != nil {
		plog.Error(fmt.Sprintf("Could not request lease from etcd, error: %s", err))
		return err
	}
	key := fmt.Sprintf("/goconserver/hosts/%s", s.host)
	_, err = cli.Put(context.TODO(), key, s.host, clientv3.WithLease(resp.ID))
	if err != nil {
		plog.Error(fmt.Sprintf("Could not update host on etcd, error: %s", err))
		return err
	}
	go func() {
		t := serverConfig.Etcd.ServerHeartbeat - 2
		if t <= 0 {
			t = 1
		}
		cli, err := s.getClient()
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

func (s *EtcdStorage) getHosts(cli *clientv3.Client) ([]string, error) {
	var err error
	if cli == nil {
		err = errors.New("Please initialize the client for etcd")
		return nil, err
	}
	resp, err := cli.Get(context.TODO(), "/goconserver/hosts", clientv3.WithPrefix())
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
	dirKey := fmt.Sprintf("/goconserver/%s/nodes", s.host)
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

func (s *EtcdStorage) getHostCount(cli *clientv3.Client) (map[string]int, error) {
	var err error
	if cli == nil {
		err = errors.New("Please initialize the client for etcd")
		return nil, err
	}
	hosts, err := s.getHosts(cli)
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

func (s *EtcdStorage) getMinHost(hostsCount map[string]int) string {
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

func (s *EtcdStorage) ListNodeWithHost() map[string]string {
	cli, err := s.getClient()
	if err != nil {
		plog.Error(err)
		return nil
	}
	defer cli.Close()
	hosts, err := s.getHosts(cli)
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
		hostsCount, err := s.getHostCount(cli)
		if err != nil {
			plog.Error(err)
			return
		}
		for _, v := range nodes.(map[string][]Node)["nodes"] {
			host := s.getMinHost(hostsCount)
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
			key := fmt.Sprintf("/goconserver/%s/nodes/%s", s.host, v)
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
	key := fmt.Sprintf("/goconserver/%s/nodes/", s.host)
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

func (s *EtcdStorage) SupportWatcher() bool {
	return true
}
