package main

import (
	"log"
	"os"

	"github.com/chenglch/consoleserver/common"
	"github.com/chenglch/consoleserver/console"
)

func main() {
	if len(os.Args) != 4 {
		log.Fatalf("Usage: %s <host> <ip> <node>", os.Args[0])
	}
	common.InitLogger()
	common.NewTaskManager(100, 16)
	client := console.NewConsoleClient(os.Args[1], os.Args[2])
	conn, err := client.Connect()
	if err != nil {
		panic(err)
	}
	client.Handle(conn, os.Args[3])
}
