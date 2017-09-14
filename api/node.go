package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/chenglch/consoleserver/common"
	"github.com/chenglch/consoleserver/service"
	"github.com/gorilla/mux"
)

var (
	nodeManager *service.NodeManager
	plog        *common.Logger
)

func init() {
	plog = common.GetLogger("github.com/chenglch/consoleserver/api/node")
}

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
	}
	api.routes = routes
	for _, route := range routes {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
	}
	nodeManager = service.NewNodeManager()
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
	var index int
	if index, err = api.index(vars["node"]); err != nil {
		plog.HandleHttp(w, req, http.StatusBadRequest, err)
		return
	}
	node := nodeManager.Nodes[index]
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if resp, err = json.Marshal(node); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s\n", resp)
}

func (api *NodeApi) post(w http.ResponseWriter, req *http.Request) {
	var node service.Node
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	if err := req.Body.Close(); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	if err := json.Unmarshal(body, &node); err != nil {
		plog.HandleHttp(w, req, http.StatusUnprocessableEntity, err)
		return
	}

	if api.exists(node) {
		err := errors.New("Already exist")
		plog.HandleHttp(w, req, http.StatusConflict, err)
		return
	}
	node.SetStatus(service.STATUS_ENROLL)
	nodeManager.Nodes = append(nodeManager.Nodes, &node)
	nodeManager.RefreshNodeMap()
	nodeManager.Save(w, req)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusCreated)
	common.GetTaskManager().Register(node.StartConsole)
}

func (api *NodeApi) delete(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	var err error
	var index int
	if index, err = api.index(vars["node"]); err != nil {
		plog.HandleHttp(w, req, http.StatusBadRequest, err)
		return
	}
	nodeManager.Nodes = append(nodeManager.Nodes[:index], nodeManager.Nodes[index+1:]...)
	nodeManager.RefreshNodeMap()
	if nodeManager.Save(w, req) != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (api *NodeApi) exists(node service.Node) bool {
	if _, ok := nodeManager.NodeMap[node.Name]; ok {
		return true
	}
	return false
}

func (api *NodeApi) index(name string) (int, error) {
	var index int
	var ok bool
	if index, ok = nodeManager.NodeMap[name]; !ok {
		return -1, errors.New("Could not be found")
	}
	return index, nil
}
