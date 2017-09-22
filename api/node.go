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
	var resp []byte
	var err error
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	nodes := make(map[string][]string)
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
		if err := node.RequireLock(false); err != nil {
			plog.HandleHttp(w, req, http.StatusConflict, err)
			return
		}
		go node.StartConsole()
		// open a new groutine to make the rest request asynchronous
		go func() {
			if err := common.TimeoutChan(node.GetReadyChan(), serverConfig.Console.TargetTimeout); err != nil {
				plog.ErrorNode(node.Name, fmt.Sprintf("Could not start console, error: %s.", err))
			}
			node.Release(false)
		}()
	} else if state == CONSOLE_OFF {
		if err := node.RequireLock(false); err != nil {
			plog.HandleHttp(w, req, http.StatusConflict, err)
			return
		}
		go func() {
			node.StopConsole()
			node.Release(false)
		}()
	}
	plog.InfoNode(node.Name, fmt.Sprintf("The console state has been changed to %s.", state))
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusAccepted)
}

func (api *NodeApi) post(w http.ResponseWriter, req *http.Request) {
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
	nodeManager.RWlock.Lock()
	defer nodeManager.RWlock.Unlock()
	if nodeManager.Exists(node.Name) {
		err := errors.New(fmt.Sprintf("THe node name %s is already exist", node.Name))
		plog.HandleHttp(w, req, http.StatusConflict, err)
		return
	}
	if err := node.Validate(); err != nil {
		plog.HandleHttp(w, req, http.StatusBadRequest, err)
		return
	}
	node.SetStatus(console.STATUS_ENROLL)
	nodeManager.Nodes[node.Name] = node
	if err := nodeManager.Save(w, req); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusCreated)
	plog.InfoNode(node.Name, "Created.")
	if node.Ondemand == false {
		if err := node.RequireLock(false); err != nil {
			plog.HandleHttp(w, req, http.StatusConflict, err)
			return
		}
		go node.StartConsole()
		// open a new groutine to make the rest request asynchronous
		go func() {
			if err = common.TimeoutChan(node.GetReadyChan(), serverConfig.Console.TargetTimeout); err != nil {
				plog.ErrorNode(node.Name, err)
			}
			node.Release(false)
		}()
	}
}

func (api *NodeApi) delete(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	var err error
	nodeManager.RWlock.Lock()
	defer nodeManager.RWlock.Unlock()
	if !nodeManager.Exists(vars["node"]) {
		plog.HandleHttp(w, req, http.StatusBadRequest, errors.New(fmt.Sprintf("Node %s is not exist", vars["node"])))
		return
	}
	node := nodeManager.Nodes[vars["node"]]
	if node.GetStatus() == console.STATUS_CONNECTED {
		if err := node.RequireLock(false); err != nil {
			plog.HandleHttp(w, req, http.StatusConflict, err)
			return
		}
		node.StopConsole()
		node.RequireLock(false)
	}
	delete(nodeManager.Nodes, vars["node"])
	if err = nodeManager.Save(w, req); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	plog.InfoNode(node.Name, "Deteled.")
	w.WriteHeader(http.StatusAccepted)
}
