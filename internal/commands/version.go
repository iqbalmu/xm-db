package commands

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		titleColor := color.New(color.FgCyan, color.Bold)
		infoColor := color.New(color.FgWhite)

		titleColor.Printf("DBM Version: ")
		infoColor.Println(Version)

		fmt.Println("Supported Providers:")
		fmt.Println("  - MySQL")
		fmt.Println("  - PostgreSQL")
	},
}
