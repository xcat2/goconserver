package console

import (
	"fmt"
	"github.com/chenglch/goconserver/common"
	"os"
)

const (
	CRLF = "\r\n"
)

func printNodeNotfoundMsg(node string) {
	if clientConfig.ClientType == common.CLIENT_XCAT_TYPE {
		fmt.Printf("Could not find node %s, did you run 'makegocons %s'? \n", node, node)
		return
	}
	fmt.Printf("Could not find node %s \n", node)
}

func printConsoleHelpPrompt() {
	fmt.Printf("[Enter `^Ec?' for help]\r\n")
}

func printConsoleReceiveErr(err error) {
	fmt.Printf("\rCould not receive message, error: %s.\r\n", err.Error())
}

func printConsoleSendErr(err error) {
	fmt.Printf("\rCould not send message, error: %s.\r\n", err.Error())
}

func printConsoleHelpMsg() {
	fmt.Printf("\r\nHelp message from congo:\r\n" +
		"Ctrl + e + c + .         Exit from console session  \r\n" +
		"Ctrl + e + c + ?         Print the help message for console command \r\n" +
		"Ctrl + e + c + r         Replay last lines \r\n" +
		"Ctrl + e + c + w         Who is on this session \r\n")
}

func printConsoleCmdErr(err error) {
	fmt.Printf("Console command err: %s", err.Error())
}

func printConsoleReplay(replay string) {
	fmt.Printf("\r\n%s\r\n", replay)
}

func printCRLF() {
	fmt.Printf(CRLF)
}

func printConsoleUser(user string) {
	fmt.Printf("User: %s\r\n", user)
}

func printFatalErr(err error) {
	fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
}

func printConsoleDisconnectPrompt() {
	fmt.Printf("[Disconnected]\r\n")
}
