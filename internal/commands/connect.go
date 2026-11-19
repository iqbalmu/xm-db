package commands

import (
	"encoding/json"
	"fmt"

	"github.com/atotto/clipboard"
	"github.com/fatih/color"
	"github.com/iqbalmu/xmdb/internal/providers"
	"github.com/iqbalmu/xmdb/internal/state"
	"github.com/spf13/cobra"
)

var (
	connectFormat string
	connectCopy   bool
)

var connectCmd = &cobra.Command{
	Use:   "connect <database-name>",
	Short: "Show connection information",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConnect(args[0])
	},
}

func init() {
	connectCmd.Flags().StringVar(&connectFormat, "format", "string", "Output format (string, json, env)")
	connectCmd.Flags().BoolVarP(&connectCopy, "copy", "c", false, "Copy connection string to clipboard")
}

func runConnect(name string) error {
	stateMgr, err := state.NewManager()
	if err != nil {
		return err
	}

	db, err := stateMgr.Load(name)
	if err != nil {
		return fmt.Errorf("database '%s' not found", name)
	}

	prov, err := providers.For(db.Provider)
	if err != nil {
		return err
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
			return err
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

	if connectCopy {
		if err := clipboard.WriteAll(connStr); err != nil {
			return fmt.Errorf("copying to clipboard: %w", err)
		}
		if connectFormat == "string" {
			color.Cyan("Copied connection string to clipboard!")
			fmt.Println()
		}
	}
	return nil
}
