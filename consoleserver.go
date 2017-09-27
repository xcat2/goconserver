package main

import (
	"log"
	"net/http"

	"fmt"

	"github.com/chenglch/consoleserver/api"
	"github.com/chenglch/consoleserver/common"
	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"os"
	"time"
)

var (
	ServerCmd *cobra.Command
	confFile  string
	showVer   bool
	Version   string
	BuildTime string
	Commit    string
)

func init() {
	ServerCmd = &cobra.Command{
		Use:   "consoleserver",
		Short: "This is consoleserver damon service",
		Long:  `Consoleserver daemon service`}
	ServerCmd.Flags().StringVarP(&confFile, "config-file", "c", "/etc/consoleserver/server.conf", "Specify the configuration file for consoleserver daemon.")
	ServerCmd.Flags().BoolVarP(&showVer, "version", "v", false, "Show the version of consoleserver.")
}

func main() {
	if err := ServerCmd.Execute(); err != nil {
		panic(err)
	}
	if showVer {
		fmt.Printf("Version: %s, BuildTime: %s Commit: %s\n", Version, BuildTime, Commit)
		os.Exit(0)
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
