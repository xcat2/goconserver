package api

import (
	"fmt"
	"net/http"

	"github.com/chenglch/goconserver/console"
	"github.com/gorilla/mux"
)

type CommandApi struct {
	routes Routes
}

func NewCommandApi(router *mux.Router) *CommandApi {
	api := CommandApi{}
	routes := Routes{
		Route{"Command", "GET", "/command/replay/{node}", api.replay},
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

func (api *CommandApi) replay(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	plog.Debug(fmt.Sprintf("Receive %s request %s %v from %s.", req.Method, req.URL.Path, vars, req.RemoteAddr))
	content, httpcode, msg := nodeManager.Replay(vars["node"])
	if httpcode >= 400 {
		plog.HandleHttp(w, req, httpcode, msg)
		return
	}
	w.Header().Set("Content-Type", "html/text; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s\n", content)
}
