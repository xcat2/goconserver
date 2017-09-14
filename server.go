package main

import (
	"log"
	"net/http"

	"github.com/chenglch/consoleserver/api"
	"github.com/chenglch/consoleserver/common"
	"github.com/gorilla/mux"
)

func main() {
	common.NewTaskManager(10000, 16)
	api.Router = mux.NewRouter().StrictSlash(true)
	api.NewNodeApi(api.Router)
	log.Fatal(http.ListenAndServe(":8089", api.Router))
}
