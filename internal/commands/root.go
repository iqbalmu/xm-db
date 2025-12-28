package commands

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/commands/provider"
	"github.com/iqbalmu/dbm/internal/commands/table"
	"github.com/spf13/cobra"
)

var Version = "1.0.0"

var rootCmd = &cobra.Command{
	Use:   "dbm",
	Short: "Database Management CLI for Development",
	Long: `DBM is a powerful CLI tool for managing development databases.
It supports MySQL and PostgreSQL with easy container management.`,
	Run: func(cmd *cobra.Command, args []string) {
		showOverview()
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(provider.Cmd)
	rootCmd.AddCommand(table.Cmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(connectCmd)
}

func showOverview() {
	titleColor := color.New(color.FgCyan, color.Bold)
	subtitleColor := color.New(color.FgWhite)
	commandColor := color.New(color.FgGreen)

	titleColor.Println("\n╔═══════════════════════════════════════════════════════╗")
	titleColor.Println("║          DBM - Database Management CLI                ║")
	titleColor.Println("╚═══════════════════════════════════════════════════════╝")

	subtitleColor.Println("\nManage your development databases with ease!")
	subtitleColor.Println("Supports: MySQL, PostgreSQL")

	fmt.Println("\n📦 Common Commands:")
	commandColor.Print("  dbm version")
	fmt.Println("              Show version information")

	commandColor.Print("  dbm status")
	fmt.Println("               Show status of all databases")

	commandColor.Print("  dbm provider list")
	fmt.Println("         List available database providers")

	commandColor.Print("  dbm create <name>")
	fmt.Println("           Create a new database")

	commandColor.Print("  dbm list")
	fmt.Println("                  List all managed databases")

	commandColor.Print("  dbm start <name>")
	fmt.Println("           Start a database")

	commandColor.Print("  dbm stop <name>")
	fmt.Println("            Stop a database")

	commandColor.Print("  dbm connect <name>")
	fmt.Println("          Show connection information")

	fmt.Println("\n💡 Quick Start:")
	fmt.Println("  1. dbm create mydb --provider mysql")
	fmt.Println("  2. dbm connect mydb")
	fmt.Println("  3. dbm table list mydb")

	fmt.Println("\n📖 For more information, run: dbm --help")
	fmt.Println()
}
