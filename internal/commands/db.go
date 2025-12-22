package commands

import (
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Manage databases",
}

func init() {
	dbCmd.AddCommand(dbCreateCmd)
	dbCmd.AddCommand(dbListCmd)
	dbCmd.AddCommand(dbStartCmd)
	dbCmd.AddCommand(dbStopCmd)
	dbCmd.AddCommand(dbRemoveCmd)
	dbCmd.AddCommand(dbInfoCmd)
}
