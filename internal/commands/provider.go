package commands

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage database providers",
}

var providerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available database providers",
	Run: func(cmd *cobra.Command, args []string) {
		runProviderList()
	},
}

func init() {
	providerCmd.AddCommand(providerListCmd)
}

func runProviderList() {
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
