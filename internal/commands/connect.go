package commands

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/state"
	"github.com/iqbalmu/dbm/pkg/utils"
	"github.com/spf13/cobra"
)

var connectFormat string

var connectCmd = &cobra.Command{
	Use:   "connect <database-name>",
	Short: "Show connection information",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runConnect(args[0])
	},
}

func init() {
	connectCmd.Flags().StringVar(&connectFormat, "format", "string", "Output format (string, json, env)")
}

func runConnect(name string) {
	stateMgr, err := state.NewManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	db, err := stateMgr.Load(name)
	if err != nil {
		color.Red("Error: Database '%s' not found", name)
		return
	}

	// Build connection string using helper methods
	connStr := utils.BuildConnectionString(
		db.Provider,
		db.GetUsername(),
		db.GetPassword(),
		"localhost",
		db.Port,
		db.DatabaseName,
	)

	switch connectFormat {
	case "json":
		fmt.Printf(`{
  "provider": "%s",
  "host": "localhost",
  "port": %d,
  "database": "%s",
  "user": "%s",
  "password": "%s",
  "connection_string": "%s"
}
`, db.Provider, db.Port, db.DatabaseName, db.User, db.UserPassword, connStr)

	case "env":
		fmt.Printf("DB_PROVIDER=%s\n", db.Provider)
		fmt.Printf("DB_HOST=localhost\n")
		fmt.Printf("DB_PORT=%d\n", db.Port)
		fmt.Printf("DB_NAME=%s\n", db.DatabaseName)
		fmt.Printf("DB_USER=%s\n", db.User)
		fmt.Printf("DB_PASSWORD=%s\n", db.UserPassword)

	default:
		color.Cyan("\n🔗 Connection Information for '%s':\n", name)
		fmt.Println(color.CyanString("════════════════════════════════════════"))

		labelColor := color.New(color.FgWhite, color.Bold)

		labelColor.Print("\nProvider:  ")
		fmt.Println(db.Provider)

		labelColor.Print("Host:      ")
		fmt.Println("localhost")

		labelColor.Print("Port:      ")
		fmt.Println(db.Port)

		labelColor.Print("Database:  ")
		fmt.Println(db.DatabaseName)

		if db.User != "" {
			labelColor.Print("User:      ")
			fmt.Println(db.User)

			labelColor.Print("Password:  ")
			fmt.Println(db.UserPassword)
		}

		fmt.Println()
		labelColor.Println("Connection String:")
		color.Green("%s", connStr)
		fmt.Println()
	}
}
