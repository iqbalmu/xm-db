package table

import (
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "table",
	Short: "Manage database tables",
}

func init() {
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(describeCmd)
}
