package db

import (
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "db",
	Short: "Manage databases",
}

func init() {
	Cmd.AddCommand(createCmd)
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(startCmd)
	Cmd.AddCommand(stopCmd)
	Cmd.AddCommand(removeCmd)
	Cmd.AddCommand(infoCmd)
	Cmd.AddCommand(logsCmd)
	Cmd.AddCommand(backupCmd)
	Cmd.AddCommand(restoreCmd)
}
