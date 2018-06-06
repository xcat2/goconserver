package etcd

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/coreos/etcd/pkg/transport"
	"github.com/xcat2/goconserver/common"
	"strings"
	"time"
)

const (
	maxOpSize = 128
)

var (
	plog = common.GetLogger("github.com/xcat2/goconserver/storage/etcd")
)

type EtcdClient struct {
	config *common.EtcdCfg
	cli    *clientv3.Client
}

func NewEtcdClient(config *common.EtcdCfg) (*EtcdClient, error) {
	var err error
	var tlsInfo transport.TLSInfo
	var tlsConfig *tls.Config
	endpoints := strings.Split(config.Endpoints, ",")
	if len(endpoints) < 1 {
		plog.Error("Invalid parameter: etcd endpoints, ip/hostname list separated with space should be specified.")
		return nil, common.ErrInvalidParameter
	}
	if config.SSLCertFile == "" && config.SSLKeyFile == "" && config.SSLCertFile == "" {
		tlsConfig = nil
	}
	tlsInfo = transport.TLSInfo{
		CertFile: config.SSLCertFile,
		KeyFile:  config.SSLKeyFile,
		CAFile:   config.SSLCertFile,
	}
	tlsConfig, err = tlsInfo.ClientConfig()
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	if config.SSLCertFile == "" || config.SSLKeyFile == "" || config.SSLCertFile == "" {
		tlsConfig = nil
	}
	etcdClient := new(EtcdClient)
	etcdClient.config = config
	etcdClient.cli, err = clientv3.New(clientv3.Config{
		Endpoints:   endpoints[:],
		DialTimeout: time.Duration(config.DailTimeout) * time.Second,
		TLS:         tlsConfig,
	})
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	return etcdClient, nil
}

func (self *EtcdClient) Close() {
	self.cli.Close()
}

func (self *EtcdClient) Get(key string) ([]byte, error) {
	var err error
	var resp *clientv3.GetResponse
	if resp, err = self.cli.Get(context.TODO(), key); err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		plog.Warn(fmt.Sprintf("Could not find key %s", key))
		return nil, common.ErrNotExist
	}
	return resp.Kvs[0].Value, nil
}

func (self *EtcdClient) Put(key string, val interface{}) error {
	var err error
	var s string
	switch val.(type) {
	case string:
		s = val.(string)
	case []byte:
		s = string(val.([]byte))
	default:
		plog.Error("Invalid type")
		return common.ErrInvalidType
	}
	if _, err = self.cli.Put(context.TODO(), key, s); err != nil {
		return err
	}
	return nil
}

func (self *EtcdClient) Del(key string) error {
	var err error
	if _, err = self.cli.Delete(context.TODO(), key); err != nil {
		return err
	}
	return nil
}

func (self *EtcdClient) List(path string) (map[string]string, error) {
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	resp, err := self.cli.Get(context.TODO(), path, clientv3.WithPrefix())
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	ret := make(map[string]string)
	for _, v := range resp.Kvs {
		ret[string(v.Key)] = string(v.Value)
	}
	return ret, nil
}

func (self *EtcdClient) Keys(path string) ([]string, error) {
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	resp, err := self.cli.Get(context.TODO(), path, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	ret := make([]string, resp.Count)
	for i := int64(0); i < resp.Count; i++ {
		ret[i] = string(resp.Kvs[i].Key)
	}
	return ret, nil
}

func (self *EtcdClient) Count(path string) (int, error) {
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	resp, err := self.cli.Get(context.TODO(), path, clientv3.WithPrefix(), clientv3.WithCountOnly())
	if err != nil {
		plog.Error(err)
		return 0, err
	}
	return int(resp.Count), nil
}

func (self *EtcdClient) MultiPut(m map[string]string) error {
	var err error
	var response *clientv3.TxnResponse
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.config.RequestTimeout)*time.Second)
	defer cancel()
	var ops [][]clientv3.Op
	block := make([]clientv3.Op, 0)
	for k, v := range m {
		block = append(block, clientv3.OpPut(k, v))
		if len(block) == maxOpSize {
			ops = append(ops, block)
			block = make([]clientv3.Op, 0)
		}
	}
	if len(block) != 0 {
		ops = append(ops, block)
	}
	for _, block := range ops {
		response, err = self.cli.Txn(ctx).Then(block...).Commit()
		if ctx.Err() == context.DeadlineExceeded {
			plog.Error("Timeout while execute multiple put operation")
			return common.ErrTimeout
		}
		if err != nil {
			plog.Error(err)
			return err
		}
		if !response.Succeeded {
			plog.Error(common.ErrETCDTransaction)
			return common.ErrETCDTransaction
		}
	}
	return nil
}

func (self *EtcdClient) MultiGet(m map[string]string) (map[string]string, error) {
	var err error
	var response *clientv3.TxnResponse
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.config.RequestTimeout)*time.Second)
	defer cancel()
	var ops [][]clientv3.Op
	block := make([]clientv3.Op, 0)
	for k, _ := range m {
		block = append(block, clientv3.OpGet(k))
		if len(block) == maxOpSize {
			ops = append(ops, block)
			block = make([]clientv3.Op, 0)
		}
	}
	if len(block) != 0 {
		ops = append(ops, block)
	}
	for _, block := range ops {
		response, err = self.cli.Txn(ctx).Then(block...).Commit()
		if ctx.Err() == context.DeadlineExceeded {
			plog.Error("Timeout while execute multiple get operation")
			return nil, common.ErrTimeout
		}
		if err != nil {
			plog.Error(err)
			return nil, err
		}
		if !response.Succeeded {
			plog.Error(common.ErrETCDTransaction)
			return nil, common.ErrETCDTransaction
		}
	}
	ret := make(map[string]string)
	for _, v := range response.Responses {
		item := v.GetResponseRange()
		if len(item.Kvs) == 0 {
			continue
		}
		kv := item.Kvs[0]
		ret[string(kv.Key)] = string(kv.Value)
	}
	return ret, nil
}

func (self *EtcdClient) MultiDel(keys []string) error {
	var err error
	var response *clientv3.TxnResponse
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(self.config.RequestTimeout)*time.Second)
	defer cancel()
	var ops [][]clientv3.Op
	block := make([]clientv3.Op, 0)
	for _, k := range keys {
		block = append(block, clientv3.OpDelete(k))
		if len(block) == maxOpSize {
			ops = append(ops, block)
			block = make([]clientv3.Op, 0)
		}
	}
	if len(block) != 0 {
		ops = append(ops, block)
	}
	for _, block := range ops {
		response, err = self.cli.Txn(ctx).Then(block...).Commit()
		if ctx.Err() == context.DeadlineExceeded {
			plog.Error("Timeout while execute multiple del operation")
			return common.ErrTimeout
		}
		if err != nil {
			plog.Error(err)
			return err
		}
		if !response.Succeeded {
			plog.Error(common.ErrETCDTransaction)
			return common.ErrETCDTransaction
		}
	}
	return nil
}

func (self *EtcdClient) Lock(path string) (*concurrency.Mutex, error) {
	session, err := concurrency.NewSession(self.cli)
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	m := concurrency.NewMutex(session, path)
	if err = m.Lock(context.TODO()); err != nil {
		plog.Error(err)
		return nil, err
	}
	return m, nil
}

func (self *EtcdClient) Unlock(m *concurrency.Mutex) error {
	return m.Unlock(context.TODO())
}

/* Register service in etcd, this method would not return if register service successfully.
   If keepalive request can not be reached in time, etcd would clear the servicePath entry. */
func (self *EtcdClient) RegisterAndKeepalive(lockPath string, servicePath string, serviceVal string, ready chan<- struct{}) error {
	m, err := self.Lock(lockPath)
	if err != nil {
		plog.Error(err)
		return err
	}
	_, err = self.Get(servicePath)
	if err == nil {
		// has value
		self.Unlock(m)
		plog.Warn(fmt.Sprintf("%s:%s", servicePath, common.ErrAlreadyExist.Error()))
		return common.ErrAlreadyExist
	}
	lease := clientv3.NewLease(self.cli)
	resp, err := lease.Grant(context.TODO(), self.config.ServiceHeartbeat)
	if err != nil {
		plog.Error(err)
		self.Unlock(m)
		return err
	}
	_, err = self.cli.Put(context.TODO(), servicePath, serviceVal, clientv3.WithLease(resp.ID))
	if err != nil {
		plog.Error(err)
		self.Unlock(m)
		return err
	}
	self.Unlock(m)
	ready <- struct{}{}
	t := self.config.ServiceHeartbeat / 2
	if t <= 0 {
		t = 1
	}
	for {
		self.cli.KeepAlive(context.TODO(), resp.ID)
		time.Sleep(time.Duration(t) * time.Second)
	}
	return nil
}

func (self *EtcdClient) Watch(path string, fc func([]*clientv3.Event, chan<- interface{}), c chan<- interface{}) {
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	ctx, cancel := context.WithCancel(context.Background())
	rch := self.cli.Watch(ctx, path, clientv3.WithPrefix())
	for resp := range rch {
		fc(resp.Events, c)
	}
	cancel()
}
