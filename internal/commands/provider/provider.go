package provider

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage database providers",
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available database providers",
	Run: func(cmd *cobra.Command, args []string) {
		runList()
	},
}

func init() {
	Cmd.AddCommand(listCmd)
}

func runList() {
	titleColor := color.New(color.FgCyan, color.Bold)
	titleColor.Println("\n📦 Available Database Providers:")

	// Prepare data
	data := [][]any{
		{"mysql", providers.DefaultMySQLVersion, providers.DefaultMySQLPort, "✓ Available"},
		{"postgres", providers.DefaultPostgresVersion, providers.DefaultPostgresPort, "✓ Available"},
	}

	fmt.Println()

	table := tablewriter.NewTable(os.Stdout)
	table.Header("Provider", "Default Version", "Default Port", "Status")
	table.Bulk(data)
	table.Render()

	fmt.Println()

	fmt.Println("Usage: xmdb create <name> --provider <provider>")
	fmt.Println("Example: xmdb create mydb --provider mysql")
	fmt.Println()
}
