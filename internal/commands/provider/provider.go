package provider

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// Cmd is the provider management command
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
		{"mysql", "8.0", 3306, "✓ Available"},
		{"postgres", "16", 5432, "✓ Available"},
	}

	fmt.Println()

	// New API v1+
	table := tablewriter.NewTable(os.Stdout)
	table.Header("Provider", "Default Version", "Default Port", "Status")
	table.Bulk(data)
	table.Render()

	fmt.Println()

	fmt.Println("Usage: dbm db create <name> --provider <provider>")
	fmt.Println("Example: dbm db create mydb --provider mysql")
	fmt.Println()
}
