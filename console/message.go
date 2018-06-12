package console

import (
	"fmt"
	"github.com/xcat2/goconserver/common"
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
		"Ctrl + e + c + .                 Exit from console session\r\n" +
		"Ctrl + e + c + ?                 Print the help message for console command\r\n" +
		"Ctrl + e + c + r                 Replay last lines (only for file_logger)\r\n" +
		"Ctrl + e + c + w                 Who is on this session\r\n" +
		"Ctrl + e + c + l + [1-9]/?       Send break sequence to the remote\r\n")
}

func printBreakSequence(last byte) {
	s := string([]byte{'^', 'E', ESCAPE_C, CLIENT_CMD_LOCAL, last})
	fmt.Printf("\r\nBreak sequence %s pressed\r\n", s)
}

func printConsoleCmdErr(msg interface{}) {
	switch t := msg.(type) {
	case string:
		fmt.Printf("\r\nConsole command err: %s\r\n", t)
	case error:
		fmt.Printf("\r\nConsole command err: %s\r\n", t.Error())
	default:
		fmt.Printf("\r\nConsole command err: unexpeted type\r\n")
	}
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

func printConsoleDisconnectPrompt(mode int, node string) {
	if mode == CLIENT_INTERACTIVE_MODE {
		fmt.Printf("[Disconnected]\r\n")
		return
	}
	fmt.Printf("%s: [Disconnected]\r\n", node)
}
