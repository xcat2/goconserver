package console

import (
	"fmt"
	"github.com/chenglch/goconserver/common"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	createParams string
	clientConfig *common.ClientConfig
)

func KeyValueArrayToMap(values []string, sep string) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	for _, value := range values {
		item := strings.Split(value, sep)
		if len(item) < 2 {
			return nil, fmt.Errorf("The format of %s is not correct.", item)
		}
		b, err := strconv.ParseBool(item[1])
		if err == nil {
			m[item[0]] = b
		} else {
			m[item[0]] = item[1]
		}
	}
	return m, nil
}

func KeyValueToMap(value string, sep string) (map[string]interface{}, error) {
	// transform the string like
	// bmc_address=11.0.0.0,bmc_password=password,bmc_username=admin
	m := make(map[string]interface{})
	temps := strings.Split(value, sep)
	for _, temp := range temps {
		item := strings.Split(temp, "=")
		if len(temp) < 2 {
			return nil, fmt.Errorf("The format of %s is not correct.", value)
		}
		b, err := strconv.ParseBool(item[1])
		if err == nil {
			m[item[0]] = b
		} else {
			m[item[0]] = item[1]
		}
	}
	return m, nil
}

type nodeHost struct {
	name string
	host string
}
type nodeHostSlice []nodeHost

func (a nodeHostSlice) Len() int {
	return len(a)
}

func (a nodeHostSlice) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a nodeHostSlice) Less(i, j int) bool {
	if strings.Compare(a[i].name, a[j].name) == -1 {
		return true
	}
	return false
}

type CongoCli struct {
	baseUrl string
	cmd     *cobra.Command
}

func NewCongoCli(cmd *cobra.Command) {
	var err error
	cli := new(CongoCli)
	clientConfig, err = common.NewClientConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load configuration error %s\n", err.Error())
		os.Exit(1)
	}
	cli.baseUrl = clientConfig.HTTPUrl
	cli.cmd = cmd
	cli.cmd.AddCommand(cli.listCommand())
	cli.cmd.AddCommand(cli.showCommand())
	cli.cmd.AddCommand(cli.loggingCommand())
	cli.cmd.AddCommand(cli.deleteCommand())
	cli.cmd.AddCommand(cli.createCommand())
	cli.cmd.AddCommand(cli.consoleCommand())
	if err := cli.cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Could not execute congo command, %s\n", err.Error())
	}
}

func (c *CongoCli) listCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List node(s) in goconserver service",
		Long:  `List node(s) in goconserver service. Format: congo list`,
		Run:   c.list,
	}
	return cmd
}

func (c *CongoCli) list(cmd *cobra.Command, args []string) {
	congo := NewCongoClient(c.baseUrl)
	nodes, err := congo.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not list resources, %s\n", err.Error())
		os.Exit(1)
	}
	if len(nodes) == 0 {
		fmt.Printf("Could not find any record.\n")
		os.Exit(0)
	}
	tempNodes := make([]nodeHost, 0, len(nodes))
	for _, v := range nodes {
		node := v.(map[string]interface{})
		tempNodes = append(tempNodes, nodeHost{name: node["name"].(string),
			host: node["host"].(string)})
	}
	sort.Sort(nodeHostSlice(tempNodes))
	for _, v := range tempNodes {
		fmt.Printf("%s (host: %s)\n", v.name, v.host)
	}
}

func (c *CongoCli) showCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <node>",
		Short: "Show node detail in goconserver service",
		Long:  `Show node detail in goconserver service. Format: congo show <node>`,
		Run:   c.show,
	}
	return cmd
}

func (c *CongoCli) show(cmd *cobra.Command, args []string) {
	congo := NewCongoClient(c.baseUrl)
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: congo show <node> \n")
		os.Exit(1)
	}
	if strings.HasPrefix(".", args[0]) || strings.HasPrefix("/", args[0]) {
		fmt.Fprintf(os.Stderr, "Error: node name could not start with '.' or '/'\n")
		os.Exit(1)
	}
	ret, err := congo.Show(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not get resources detail, %s\n", err.Error())
		os.Exit(1)
	}
	common.PrintJson(ret.([]byte))
}

func (c *CongoCli) loggingCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logging <node> on/off",
		Short: "Enable or disable the logging for specified node",
		Long: `Enable or disable the console looging for the node. If on, the
connection will be established and start the logging, otherwise the connection will be disconnected.
Format: congo logging <node> on/off`,
		Run: c.logging,
	}
	return cmd
}

func (c *CongoCli) logging(cmd *cobra.Command, args []string) {
	congo := NewCongoClient(c.baseUrl)
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: congo logging <node> on/off \n")
		os.Exit(1)
	}
	if strings.HasPrefix(".", args[0]) || strings.HasPrefix("/", args[0]) {
		fmt.Fprintf(os.Stderr, "Error: node name could not start with '.' or '/'\n")
		os.Exit(1)
	}
	if args[1] != "on" && args[1] != "off" {
		fmt.Fprintf(os.Stderr, "Usage: congo logging <node> on/off \n")
		os.Exit(1)
	}
	_, err := congo.Logging(args[0], args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
}

func (c *CongoCli) deleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <node>",
		Short: "Delete node in console server",
		Long:  `Delete node in console server. Format: congo delete <node>`,
		Run:   c.delete,
	}
	return cmd
}

func (c *CongoCli) delete(cmd *cobra.Command, args []string) {
	congo := NewCongoClient(c.baseUrl)
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: congo delete <node>\n")
		os.Exit(1)
	}
	if strings.HasPrefix(".", args[0]) || strings.HasPrefix("/", args[0]) {
		fmt.Fprintf(os.Stderr, "Error: node name could not start with '.' or '/'\n")
		os.Exit(1)
	}
	_, err := congo.Delete(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("Deleted\n")
}

func (c *CongoCli) createCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <node>",
		Short: "Create node in console server",
		Long: `Create node in console server. Format: congo create <node> driver=ssh ondemand=true --params=<key=val>[,<key>=<val>]
Example: congo create kvmguest1 driver=cmd ondemand=false --params cmd="/opt/xcat/share/xcat/cons/kvm kvmguest1"
         congo create sshnode1 driver=ssh ondemand=true --params user=root,host=11.5.102.73,port=22,password=cluster
         congo create sshnode2 driver=ssh ondemand=false --params user=root,host=11.5.102.73,port=22,private_key=/root/.ssh/id_rsa`,
		Run: c.create,
	}
	cmd.Flags().StringVarP(&createParams, "params", "p", "",
		`Key/value pairs split by comma used by the ssh plugin, such as
			host=11.0.0.0,password=password,user=admin,port=22
			cmd="ssh -l root 11.5.102.73`)
	return cmd
}

func (c *CongoCli) create(cmd *cobra.Command, args []string) {
	congo := NewCongoClient(c.baseUrl)
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: congo create <node> driver=ssh ondemand=true --param key=val,key=val\n")
		os.Exit(1)
	}
	if strings.HasPrefix(".", args[0]) || strings.HasPrefix("/", args[0]) {
		fmt.Fprintf(os.Stderr, "Error: node name could not start with '.' or '/'\n")
		os.Exit(1)
	}
	attribs, err := KeyValueArrayToMap(args[1:], "=")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse failed, err=%s\n", err.Error())
		os.Exit(1)
	}
	params, err := KeyValueToMap(createParams, ",")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse failed, err=%s\n", err.Error())
		os.Exit(1)
	}
	_, err = congo.Create(args[0], attribs, params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("Created\n")
}

func (c *CongoCli) consoleCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "console <node>",
		Short: "Connect to the console server to start the terminal session",
		Long: `Connect to the console server to start the terminal session. Format: congo console <node>.
Note: The console connection will not be shutdown until enter the sequence keys 'ctrl+e+c+.'`,
		Run: c.console,
	}
	return cmd
}

func (c *CongoCli) waitInput(args interface{}) {
	var err error
	var exit bool
	client := args.(*ConsoleClient)
	b := make([]byte, 1024)
	in := int(os.Stdin.Fd())
	n := 0
	client.origState, err = terminal.MakeRaw(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
	exit = false
	defer func() {
		terminal.Restore(int(os.Stdin.Fd()), client.origState)
		if exit == true {
			if err == nil {
				fmt.Printf("Disconnected\n")
				os.Exit(0)
			} else {
				fmt.Fprintf(os.Stderr, err.Error())
				os.Exit(1)
			}
		}
	}()
	err = common.Fcntl(in, syscall.F_SETFL, syscall.O_ASYNC|syscall.O_NONBLOCK)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		exit = true
		return
	}
	if runtime.GOOS != "darwin" {
		err = common.Fcntl(in, syscall.F_SETOWN, syscall.Getpid())
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			exit = true
			return
		}
	}
	select {
	case _, ok := <-client.sigio:
		if !ok {
			return
		}
		for {
			size, err := syscall.Read(in, b[n:])
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				break
			}
			n += size
		}
		if err != nil && err != syscall.EAGAIN && err != syscall.EWOULDBLOCK {
			fmt.Fprintf(os.Stderr, err.Error())
			exit = true
			return
		}
		exit, _ = client.checkEscape(b, n, "")
	}
}

func (c *CongoCli) console(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: congo console <node>\n")
		os.Exit(1)
	}
	if strings.HasPrefix(".", args[0]) || strings.HasPrefix("/", args[0]) {
		fmt.Fprintf(os.Stderr, "Error: node name could not start with '.' or '/'\n")
		os.Exit(1)
	}
	retry := true
	common.NewTaskManager(100, 16)
	for retry {
		client := NewConsoleClient(clientConfig.ServerHost, clientConfig.ConsolePort)
		quit := make(chan struct{}, 0)
		client.registerSignal(quit)
		conn, err := client.Connect()
		if err != nil {
			fmt.Printf("\rCould not connect to %s\n", args[0])
			panic(err)
		}
		host, err := client.Handle(conn, args[0])
		if err == nil && host != "" {
			client = NewConsoleClient(host, clientConfig.ConsolePort)
			conn, err = client.Connect()
			if err != nil {
				panic(err)
			}
			_, err = client.Handle(conn, args[0])
		}
		if err != nil {
			fmt.Printf("\rThe connection is disconnected\n")
		}
		if client.retry {
			fmt.Println("[Enter `^Ec.' to exit]\r\nSession is teminated unexpectedly, retrying....")
			client.sigio = make(chan struct{}, 1)
			waitTask, err := common.GetTaskManager().RegisterLoop(c.waitInput, client)
			if err != nil {
				fmt.Fprintf(os.Stderr, err.Error())
				os.Exit(1)
			}
			time.Sleep(time.Duration(10) * time.Second)
			common.GetTaskManager().Stop(waitTask.GetID())
			common.SafeClose(client.sigio)
			quit <- struct{}{}
		}
		close(quit)
		retry = client.retry
	}
}
