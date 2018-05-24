package api

import (
	"fmt"
	"net/http"

	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/xcat2/goconserver/console"
)

type EscapeApi struct {
	routes Routes
}

func NewEscapeApi(router *mux.Router) *CommandApi {
	api := CommandApi{}
	routes := Routes{
		Route{"Command", "GET", "/breaksequence", api.listSequence},
	}
	api.routes = routes
	for _, route := range routes {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
	}
	return &api
}

func (api *CommandApi) listSequence(w http.ResponseWriter, req *http.Request) {
	plog.Debug(fmt.Sprintf("Receive %s request %s from %s.", req.Method, req.URL.Path, req.RemoteAddr))
	var resp []byte
	var err error
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	serverEscape := console.GetServerEscape()
	if serverEscape == nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err.Error())
		return
	}
	seqs := serverEscape.GetSequences()
	if resp, err = json.Marshal(seqs); err != nil {
		plog.HandleHttp(w, req, http.StatusInternalServerError, err.Error())
		return
	}
	fmt.Fprintf(w, "%s\n", resp)
}
