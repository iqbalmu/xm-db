package commands

import (
	"encoding/json"
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
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

	prov, err := providers.For(db.Provider)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	connStr := prov.GetConnectionString(connectionConfig(db))

	switch connectFormat {
	case "json":
		info := struct {
			Provider         string `json:"provider"`
			Host             string `json:"host"`
			Port             int    `json:"port"`
			Database         string `json:"database"`
			User             string `json:"user"`
			Password         string `json:"password"`
			ConnectionString string `json:"connection_string"`
		}{
			Provider:         db.Provider,
			Host:             "localhost",
			Port:             db.Port,
			Database:         db.DatabaseName,
			User:             db.GetUsername(),
			Password:         db.GetPassword(),
			ConnectionString: connStr,
		}

		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			color.Red("Error: %v", err)
			return
		}
		fmt.Println(string(data))

	case "env":
		fmt.Printf("DB_PROVIDER=%s\n", db.Provider)
		fmt.Printf("DB_HOST=localhost\n")
		fmt.Printf("DB_PORT=%d\n", db.Port)
		fmt.Printf("DB_NAME=%s\n", db.DatabaseName)
		fmt.Printf("DB_USER=%s\n", db.GetUsername())
		fmt.Printf("DB_PASSWORD=%s\n", db.GetPassword())

	default:
		color.Cyan("\nConnection Information for '%s':\n", name)
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

		labelColor.Print("User:      ")
		fmt.Println(db.GetUsername())

		labelColor.Print("Password:  ")
		fmt.Println(db.GetPassword())

		fmt.Println()
		labelColor.Println("Connection String:")
		color.Green("%s", connStr)
		fmt.Println()
	}
}
