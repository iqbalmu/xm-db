package table

import (
	"github.com/spf13/cobra"
)

// Cmd is the table management command
var Cmd = &cobra.Command{
	Use:   "table",
	Short: "Manage database tables",
}

func init() {
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(describeCmd)
}
