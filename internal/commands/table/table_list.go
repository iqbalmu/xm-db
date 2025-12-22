package table

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/providers"
	"github.com/iqbalmu/dbm/internal/state"
	"github.com/iqbalmu/dbm/pkg/utils"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list <database-name>",
	Aliases: []string{"ls"},
	Short:   "List all tables in a database",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runList(args[0])
	},
}

func runList(name string) {
	stateMgr, err := state.NewManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	dbState, err := stateMgr.Load(name)
	if err != nil {
		color.Red("Error: Database '%s' not found", name)
		return
	}

	// Get provider
	var prov providers.Provider
	switch dbState.Provider {
	case "mysql":
		prov = providers.NewMySQLProvider()
	case "postgres":
		prov = providers.NewPostgresProvider()
	default:
		color.Red("Error: Unsupported provider")
		return
	}

	// Connect to database
	dbConfig := &providers.DBConfig{
		Host:         "localhost",
		Port:         dbState.Port,
		RootPassword: dbState.RootPassword,
		DatabaseName: dbState.DatabaseName,
	}

	connStr := prov.GetConnectionString(dbConfig)
	db, err := sql.Open(utils.GetDriverName(dbState.Provider), connStr) // PAKAI UTILS
	if err != nil {
		color.Red("Error connecting to database: %v", err)
		return
	}
	defer db.Close()

	ctx := context.Background()

	// List tables
	tables, err := prov.ListTables(ctx, db, dbState.DatabaseName)
	if err != nil {
		color.Red("Error listing tables: %v", err)
		return
	}

	if len(tables) == 0 {
		color.Yellow("\nNo tables found in database '%s'", name)
		fmt.Println()
		return
	}

	color.Cyan("\n📋 Tables in '%s':\n", name)
	for i, table := range tables {
		fmt.Printf("  %d. %s\n", i+1, table)
	}
	fmt.Println()
}
