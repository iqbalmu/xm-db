package table

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
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

	prov, err := providers.For(dbState.Provider)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	dbConfig := &providers.DBConfig{
		Host:         "localhost",
		Port:         dbState.Port,
		RootPassword: dbState.RootPassword,
		DatabaseName: dbState.DatabaseName,
	}

	db, err := prov.Connect(dbConfig)
	if err != nil {
		color.Red("Error connecting to database: %v", err)
		return
	}
	defer db.Close()

	ctx := context.Background()

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

	color.Cyan("\nTables in '%s':\n", name)
	for i, table := range tables {
		fmt.Printf("  %d. %s\n", i+1, table)
	}
	fmt.Println()
}
