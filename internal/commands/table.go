package commands

import (
	"github.com/spf13/cobra"
)

var tableCmd = &cobra.Command{
	Use:   "table",
	Short: "Manage database tables",
}

func init() {
	tableCmd.AddCommand(tableListCmd)
	tableCmd.AddCommand(tableDescribeCmd)
}
