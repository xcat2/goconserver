package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/xcat2/goconserver/console"
)

var (
	Version   string
	BuildTime string
	Commit    string
)

func versionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number of congo",
		Long:  `Print the version number of congo`,
		Run:   version,
	}
	return cmd
}

func version(cmd *cobra.Command, args []string) {
	fmt.Printf("Version: %s, BuildTime: %s\n Commit: %s\n", Version, BuildTime, Commit)
}

func main() {
	cmd := &cobra.Command{
		Use:   "congo",
		Short: "This is golang client for goconserver",
		Long: `Congo --help and congo help COMMAND to see the usage for specfied
	command.`}
	cmd.AddCommand(versionCommand())
	console.NewCongoCli(cmd)
}
