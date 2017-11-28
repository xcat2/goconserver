package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/chenglch/goconserver/api"
	"github.com/chenglch/goconserver/common"
	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	ServerCmd *cobra.Command
	confFile  string
	showVer   bool
	help      bool
	Version   string
	BuildTime string
	Commit    string
	sslEnable = false
)

func init() {
	ServerCmd = &cobra.Command{
		Use:   "goconserver",
		Short: "This is goconserver damon service",
		Long:  `goconserver daemon service`}
	ServerCmd.Flags().StringVarP(&confFile, "config-file", "c", "/etc/goconserver/server.conf", "Specify the configuration file for goconserver daemon.")
	ServerCmd.Flags().BoolVarP(&showVer, "version", "v", false, "Show the version of goconserver.")
	ServerCmd.Flags().BoolVarP(&help, "help", "h", false, "Show the help message of goconserver.")
}

func loadTlsConfig(serverConfig *common.ServerConfig) *tls.Config {
	pool := x509.NewCertPool()
	caCertPath := serverConfig.Global.SSLCACertFile
	caCrt, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		sslEnable = false
		return nil
	}
	pool.AppendCertsFromPEM(caCrt)
	sslEnable = true
	return &tls.Config{ClientCAs: pool,
		ClientAuth:               tls.RequireAndVerifyClientCert,
		CipherSuites:             common.CIPHER_SUITES,
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true}
}

func main() {
	if err := ServerCmd.Execute(); err != nil {
		panic(err)
	}
	if showVer {
		fmt.Printf("Version: %s, BuildTime: %s Commit: %s\n", Version, BuildTime, Commit)
		os.Exit(0)
	}
	if help {
		fmt.Printf(`Usage: goconserver [flag]
	-c --config-file <configuration file>  Start the goconserver daemon service.
					       If not specified, use /etc/goconserver/server.conf by default.
	-h --help                              Show the help message.
	-v --version                           Show the version information of the goconserver.`)
		fmt.Println("\n")
		os.Exit(0)
	}
	serverConfig, err := common.InitServerConfig(confFile)
	if err != nil {
		panic(err)
	}
	common.InitLogger()
	api.Router = mux.NewRouter().StrictSlash(true)
	api.NewNodeApi(api.Router)
	api.NewCommandApi(api.Router)
	httpServer := &http.Server{
		ReadTimeout:  time.Duration(serverConfig.API.HttpTimeout) * time.Second,
		WriteTimeout: time.Duration(serverConfig.API.HttpTimeout) * time.Second,
		Addr:         fmt.Sprintf("%s:%s", serverConfig.Global.Host, serverConfig.API.Port),
		Handler:      api.Router,
	}
	if serverConfig.Global.SSLKeyFile != "" && serverConfig.Global.SSLCertFile != "" && serverConfig.Global.SSLCACertFile != "" {
		tlsConfig := loadTlsConfig(serverConfig)
		if sslEnable {
			httpServer.TLSConfig = tlsConfig
		}
	}
	if sslEnable {
		log.Fatal(httpServer.ListenAndServeTLS(serverConfig.Global.SSLCertFile, serverConfig.Global.SSLKeyFile))
	} else {
		log.Fatal(httpServer.ListenAndServe())
	}
}
