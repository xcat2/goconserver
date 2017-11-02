package main

import (
	"log"
	"net/http"

	"fmt"

	"crypto/tls"
	"crypto/x509"
	"github.com/chenglch/goconserver/api"
	"github.com/chenglch/goconserver/common"
	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"io/ioutil"
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
	sslEnable = false
)

func init() {
	ServerCmd = &cobra.Command{
		Use:   "goconserver",
		Short: "This is goconserver damon service",
		Long:  `goconserver daemon service`}
	ServerCmd.Flags().StringVarP(&confFile, "config-file", "c", "/etc/goconserver/server.conf", "Specify the configuration file for goconserver daemon.")
	ServerCmd.Flags().BoolVarP(&showVer, "version", "v", false, "Show the version of goconserver.")
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
