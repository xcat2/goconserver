package storage

import (
	"github.com/xcat2/goconserver/common"
)

type Dispatcher struct {
	stor StorInterface
}

func newDispatcher(stor StorInterface) *Dispatcher {
	return &Dispatcher{stor: stor}
}

func (self *Dispatcher) PeekPutHost(node *Node) (string, error) {
	if node.Labels != nil {
		if host, ok := node.Labels["host"]; ok {
			return host, nil
		}
	}
	m, err := self.stor.GetNodeCountEachHost()
	if err != nil {
		return "", err
	}
	host := self.getMinHost(m)
	return host, nil
}

/* peek hosts for a collection of nodes, return map(node->host) */
func (self *Dispatcher) PeekPutHostMap(storNodes []Node) (map[*Node]string, error) {
	alreadyMap, err := self.stor.ListNodeWithHost()
	if err != nil {
		return nil, err
	}
	m := make(map[*Node]string)
	waitCount := 0
	for i := 0; i < len(storNodes); i++ {
		if _, exist := alreadyMap[storNodes[i].Name]; exist {
			plog.WarningNode(storNodes[i].Name, common.ErrAlreadyExist)
			continue
		}
		if storNodes[i].Labels != nil {
			if host, ok := storNodes[i].Labels["host"]; ok {
				m[&storNodes[i]] = host
			}
		}
		waitCount++
	}
	if len(storNodes) == len(m) {
		return m, nil
	}
	counts, err := self.stor.GetNodeCountEachHost() // map(host->count)
	if err != nil {
		return nil, err
	}
	if len(counts) == 0 {
		return nil, common.ErrHostNotFound
	}
	for _, host := range m {
		counts[host]++
	}
	sum := 0
	i := 0
	hosts := make([]string, len(counts))
	for host, count := range counts {
		sum += count
		hosts[i] = host
		i++
	}
	k := 0
	perHostCount := (sum + waitCount) / len(counts)
	for i = 0; i < len(storNodes); i++ {
		if _, exist := alreadyMap[storNodes[i].Name]; exist {
			continue
		}
		if _, exist := m[&storNodes[i]]; exist {
			continue
		}
		if counts[hosts[k]] >= perHostCount && k < len(counts)-1 {
			k++
		}
		m[&storNodes[i]] = hosts[k]
		counts[hosts[k]]++
	}
	if len(m) == 0 {
		return nil, common.ErrAlreadyExist
	}
	return m, nil
}

func (self *Dispatcher) PeekDelHost(node string) (string, error) {
	var host string
	var exist bool
	m, err := self.stor.ListNodeWithHost()
	if err != nil {
		return "", err
	}
	if host, exist = m[node]; !exist {
		plog.ErrorNode(node, common.ErrNotExist)
		return "", common.ErrNotExist
	}
	return host, nil
}

func (self *Dispatcher) PeekDelHostMap(nodes []string) (map[string]string, error) { // node name->host
	var host string
	var exist bool
	m, err := self.stor.ListNodeWithHost()
	if err != nil {
		return nil, err
	}
	ret := make(map[string]string)
	for _, node := range nodes {
		if host, exist = m[node]; !exist {
			plog.WarningNode(node, common.ErrNotExist)
			continue
		}
		ret[node] = host
	}
	if len(ret) == 0 {
		return nil, common.ErrNodeNotExist
	}
	return ret, nil
}

func (self *Dispatcher) getMinHost(hostsCount map[string]int) string {
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
