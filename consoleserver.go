package main

import (
	"log"
	"net/http"

	"fmt"

	"github.com/chenglch/consoleserver/api"
	"github.com/chenglch/consoleserver/common"
	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"time"
)

var (
	ServerCmd *cobra.Command
	confFile  string
)

func init() {
	ServerCmd = &cobra.Command{
		Use:   "consoleserver",
		Short: "This is consoleserver damon service",
		Long:  `Start consoleserver daemon as the console service`}
	ServerCmd.Flags().StringVarP(&confFile, "config-file", "c", "/etc/consoleserver/server.conf", "Specify the configuration file for consoleserver daemon.")
}

func main() {
	if err := ServerCmd.Execute(); err != nil {
		panic(err)
	}
	serverConfig, err := common.InitServerConfig(confFile)
	if err != nil {
		panic(err)
	}
	common.InitLogger()
	api.Router = mux.NewRouter().StrictSlash(true)
	api.NewNodeApi(api.Router)
	httpServer := &http.Server{
		ReadTimeout:  time.Duration(serverConfig.API.HttpTimeout) * time.Second,
		WriteTimeout: time.Duration(serverConfig.API.HttpTimeout) * time.Second,
		Addr:         fmt.Sprintf("%s:%s", serverConfig.Global.Host, serverConfig.API.Port),
		Handler:      api.Router,
	}
	log.Fatal(httpServer.ListenAndServe())
}
