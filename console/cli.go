package console

import (
	"fmt"
	"github.com/chenglch/goconserver/common"
	"github.com/spf13/cobra"
	"os"
	"sort"
	"strconv"
	"strings"
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
		Long:  `List node(s) in goconserver service`,
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
		Short: "show node detail in goconserver service",
		Long:  `show node detail in goconserver service. congo show <node>`,
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
		Short: "Start or stop logging for request node",
		Long: `Turn on or turn off the console session in the background. If on, the
		        console log will be kept. congo logging <node> on/off`,
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
		Short: "delete node in console server",
		Long:  `delete node in console server. congo delete <node>`,
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
		Short: "create node in console server",
		Long:  `create node in console server. congo create <node> driver=ssh ondemand=true params=`,
		Run:   c.create,
	}
	cmd.Flags().StringVarP(&createParams, "params", "p", "",
		`Key/value pairs split by comma used by the ssh plugin, such as
		host=11.0.0.0,password=password,user=admin,port=22`)
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
		Short: "connect to the console server as the console client",
		Long: `connect to the console server as the console client. congo console <node>.
		        The console connection will not be shutdown until enter the sequence keys 'ctrl+e+c+.'`,
		Run: c.console,
	}
	return cmd
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
		conn, err := client.Connect()
		if err != nil {
			fmt.Printf("\rCould not connect to %s\n", args[0])
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
			fmt.Println("Session is teminated unexpectedly, retrying....")
			time.Sleep(time.Duration(10) * time.Second)
		}
		retry = client.retry
	}
}
