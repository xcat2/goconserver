package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/chenglch/consoleserver/common"
	"github.com/chenglch/consoleserver/console"
	"github.com/gorilla/mux"
)

const (
	CONSOLE_ON  = "on"
	CONSOLE_OFF = "off"
)

var (
	nodeManager  *console.NodeManager
	plog         = common.GetLogger("github.com/chenglch/consoleserver/api/node")
	serverConfig = common.GetServerConfig()
)

type NodeApi struct {
	routes Routes
}

func NewNodeApi(router *mux.Router) *NodeApi {
	api := NodeApi{}
	routes := Routes{
		Route{"Node", "GET", "/nodes", api.list},
		Route{"Node", "POST", "/nodes", api.post},
		Route{"Node", "GET", "/nodes/{node}", api.show},
		Route{"Node", "DELETE", "/nodes/{node}", api.delete},
		Route{"Node", "PUT", "/nodes/{node}", api.put},
		Route{"Node", "POST", "/bulk/nodes", api.bulkPost},
		Route{"Node", "DELETE", "/bulk/nodes", api.bulkDelete},
		Route{"Node", "PUT", "/bulk/nodes", api.bulkPut},
	}
	api.routes = routes
	for _, route := range routes {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
	}
	nodeManager = console.GetNodeManager()
	return &api
}

func (api *NodeApi) list(w http.ResponseWriter, req *http.Request) {
	plog.Debug(fmt.Sprintf("Receive %s request %s from %s.", req.Method, req.URL.Path, req.RemoteAddr))
	var resp []byte
	var err error
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	nodes := make(map[string][]string)
	nodes["nodes"] = make([]string, 0)
	for _, node := range nodeManager.Nodes {
		nodes["nodes"] = append(nodes["nodes"], node.Name)
	}
	if resp, err = json.Marshal(nodes); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	fmt.Fprintf(w, "%s\n", resp)
}

func (api *NodeApi) show(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	plog.Debug(fmt.Sprintf("Receive %s request %s %v from %s.", req.Method, req.URL.Path, vars, req.RemoteAddr))
	var resp []byte
	var err error
	if !nodeManager.Exists(vars["node"]) {
		err = errors.New(fmt.Sprintf("The node %s is not exist.", vars["node"]))
		plog.HandleHttp(w, req, http.StatusBadRequest, err)
		return
	}
	node := nodeManager.Nodes[vars["node"]]
	if err := node.RequireLock(true); err != nil {
		plog.HandleHttp(w, req, http.StatusConflict, err)
		return
	}
	node.State = console.STATUS_MAP[node.GetStatus()]
	node.Release(true)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if resp, err = json.Marshal(node); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s\n", resp)
}

func (api *NodeApi) put(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	plog.Debug(fmt.Sprintf("Receive %s request %s %v from %s.", req.Method, req.URL.Path, vars, req.RemoteAddr))
	var err error
	if !nodeManager.Exists(vars["node"]) {
		plog.HandleHttp(w, req, http.StatusBadRequest, err)
		return
	}
	if _, ok := req.URL.Query()["state"]; !ok {
		err = errors.New("Clould not locate the state parameters from URL")
		plog.HandleHttp(w, req, http.StatusBadRequest, err)
		return
	}
	state := req.URL.Query()["state"][0]
	nodeManager.RWlock.RLock()
	node := nodeManager.Nodes[vars["node"]]
	nodeManager.RWlock.RUnlock()
	if state == CONSOLE_ON && node.GetStatus() != console.STATUS_CONNECTED {
		api.startConsole(node)
	} else if state == CONSOLE_OFF && node.GetStatus() == console.STATUS_CONNECTED {
		api.stopConsole(node)
	}
	plog.InfoNode(node.Name, fmt.Sprintf("The console state has been changed to %s.", state))
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusAccepted)
}

func (api *NodeApi) bulkPut(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	plog.Debug(fmt.Sprintf("Receive %s request %s %v from %s.", req.Method, req.URL.Path, vars, req.RemoteAddr))
	var err error
	var resp []byte
	if _, ok := req.URL.Query()["state"]; !ok {
		err = errors.New("Clould not locate the state parameters from URL")
		plog.HandleHttp(w, req, http.StatusBadRequest, err)
		return
	}
	state := req.URL.Query()["state"][0]
	nodes := make(map[string][]console.Node, 0)
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	if err := req.Body.Close(); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	if err := json.Unmarshal(body, &nodes); err != nil {
		plog.HandleHttp(w, req, http.StatusUnprocessableEntity, err)
		return
	}
	result := make(map[string]string)
	for _, v := range nodes["nodes"] {
		if v.Name == "" {
			plog.Error("Skip this record as node name is not defined.")
			continue
		}
		nodeManager.RWlock.Lock()
		if !nodeManager.Exists(v.Name) {
			msg := "Skip this node as node is not exist."
			plog.ErrorNode(v.Name, msg)
			result[v.Name] = msg
			nodeManager.RWlock.Unlock()
			continue
		}
		node := nodeManager.Nodes[v.Name]
		nodeManager.RWlock.Unlock()
		if state == CONSOLE_ON && node.GetStatus() != console.STATUS_CONNECTED {
			api.startConsole(node)
		} else if state == CONSOLE_OFF && node.GetStatus() == console.STATUS_CONNECTED {
			api.stopConsole(node)
		}
		result[v.Name] = "Updated"
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if resp, err = json.Marshal(result); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, "%s\n", resp)
}

func (api *NodeApi) post(w http.ResponseWriter, req *http.Request) {
	plog.Debug(fmt.Sprintf("Receive %s request %s from %s.", req.Method, req.URL.Path, req.RemoteAddr))
	node := console.NewNode()
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	if err := req.Body.Close(); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	if err := json.Unmarshal(body, node); err != nil {
		plog.HandleHttp(w, req, http.StatusUnprocessableEntity, err)
		return
	}
	if node.Name == "" {
		plog.HandleHttp(w, req, http.StatusBadRequest, errors.New("Skip this record as node name is not defined"))
		return
	}
	if node.Driver == "" {
		plog.HandleHttp(w, req, http.StatusBadRequest, errors.New("Driver is not defined"))
		return
	}
	if err := node.Validate(); err != nil {
		plog.HandleHttp(w, req, http.StatusBadRequest, err)
		return
	}
	nodeManager.RWlock.Lock()
	if nodeManager.Exists(node.Name) {
		err := errors.New(fmt.Sprintf("The node name %s is already exist", node.Name))
		plog.HandleHttp(w, req, http.StatusAlreadyReported, err)
		nodeManager.RWlock.Unlock()
		return
	}
	node.SetStatus(console.STATUS_ENROLL)
	nodeManager.Nodes[node.Name] = node
	nodeManager.RWlock.Unlock()
	nodeManager.MakePersist()
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusCreated)
	plog.InfoNode(node.Name, "Created.")
	if node.Ondemand == false {
		api.startConsole(node)
	}
}

func (api *NodeApi) bulkPost(w http.ResponseWriter, req *http.Request) {
	plog.Debug(fmt.Sprintf("Receive %s request %s from %s.", req.Method, req.URL.Path, req.RemoteAddr))
	var resp []byte
	nodes := make(map[string][]console.Node, 0)
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	if err := req.Body.Close(); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	if err := json.Unmarshal(body, &nodes); err != nil {
		plog.HandleHttp(w, req, http.StatusUnprocessableEntity, err)
		return
	}
	result := make(map[string]string)
	for _, v := range nodes["nodes"] {
		// the silce pointer will be changed with the for loop, create a new variable.
		node := v
		node.Init()
		if node.Name == "" {
			plog.Error("Skip this record as node name is not defined.")
			continue
		}
		if node.Driver == "" {
			msg := "Driver is not defined."
			plog.ErrorNode(node.Name, msg)
			result[node.Name] = msg
			continue
		}
		if err := node.Validate(); err != nil {
			msg := "Failed to validate the node property."
			plog.ErrorNode(node.Name, msg)
			result[node.Name] = msg
			continue
		}
		nodeManager.RWlock.Lock()
		if nodeManager.Exists(node.Name) {
			msg := "Skip this node as node is exist."
			plog.ErrorNode(node.Name, msg)
			result[node.Name] = msg
			nodeManager.RWlock.Unlock()
			continue
		}
		node.SetStatus(console.STATUS_ENROLL)
		nodeManager.Nodes[node.Name] = &node
		nodeManager.RWlock.Unlock()
		result[node.Name] = "Created"
		if node.Ondemand == false {
			api.startConsole(&node)
		}
	}
	nodeManager.MakePersist()
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if resp, err = json.Marshal(result); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "%s\n", resp)
}

func (api *NodeApi) delete(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	plog.Debug(fmt.Sprintf("Receive %s request %s %v from %s.", req.Method, req.URL.Path, vars, req.RemoteAddr))
	nodeManager.RWlock.Lock()
	if !nodeManager.Exists(vars["node"]) {
		plog.HandleHttp(w, req, http.StatusBadRequest, errors.New(fmt.Sprintf("Node %s is not exist", vars["node"])))
		nodeManager.RWlock.Unlock()
		return
	}
	node := nodeManager.Nodes[vars["node"]]
	if node.GetStatus() == console.STATUS_CONNECTED {
		go api.stopConsole(node)
	}
	delete(nodeManager.Nodes, vars["node"])
	nodeManager.RWlock.Unlock()
	nodeManager.MakePersist()
	plog.InfoNode(node.Name, "Deteled.")
	w.WriteHeader(http.StatusAccepted)
}

func (api *NodeApi) bulkDelete(w http.ResponseWriter, req *http.Request) {
	plog.Debug(fmt.Sprintf("Receive %s request %s from %s.", req.Method, req.URL.Path, req.RemoteAddr))
	var resp []byte
	nodes := make(map[string][]console.Node, 0)
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	if err := req.Body.Close(); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	if err := json.Unmarshal(body, &nodes); err != nil {
		plog.HandleHttp(w, req, http.StatusUnprocessableEntity, err)
		return
	}
	result := make(map[string]string)
	for _, v := range nodes["nodes"] {
		if v.Name == "" {
			plog.Error("Skip this record as node name is not defined.")
			continue
		}
		nodeManager.RWlock.Lock()
		if !nodeManager.Exists(v.Name) {
			msg := "Skip this node as node is not exist."
			plog.ErrorNode(v.Name, msg)
			result[v.Name] = msg
			nodeManager.RWlock.Unlock()
			continue
		}
		node := nodeManager.Nodes[v.Name]
		if node.GetStatus() == console.STATUS_CONNECTED {
			go api.stopConsole(node)
		}
		delete(nodeManager.Nodes, v.Name)
		nodeManager.RWlock.Unlock()
		result[v.Name] = "Deleted"
	}
	nodeManager.MakePersist()
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if resp, err = json.Marshal(result); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s\n", resp)
}

func (api *NodeApi) startConsole(node *console.Node) {
	if err := node.RequireLock(false); err != nil {
		plog.ErrorNode(node.Name, fmt.Sprintf("Could not start console, error: %s", err.Error()))
		return
	}
	if node.GetStatus() == console.STATUS_CONNECTED {
		node.Release(false)
		return
	}
	go node.StartConsole()
	// open a new groutine to make the rest request asynchronous
	go func() {
		if err := common.TimeoutChan(node.GetReadyChan(), serverConfig.Console.TargetTimeout); err != nil {
			plog.ErrorNode(node.Name, err)
		}
		node.Release(false)
	}()
}

func (api *NodeApi) stopConsole(node *console.Node) {
	if err := node.RequireLock(false); err != nil {
		nodeManager.RWlock.Unlock()
		return
	}
	if node.GetStatus() == console.STATUS_CONNECTED {
		node.StopConsole()
	}
	node.Release(false)
}
