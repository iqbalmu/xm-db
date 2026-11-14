package table

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/iqbalmu/xm-db/pkg/utils"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var describeCmd = &cobra.Command{
	Use:     "describe <database-name> <table-name>",
	Aliases: []string{"desc"},
	Short:   "Describe table structure",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runDescribe(args[0], args[1])
	},
}

func runDescribe(dbName, tableName string) {
	stateMgr, err := state.NewManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	dbState, err := stateMgr.Load(dbName)
	if err != nil {
		color.Red("Error: Database '%s' not found", dbName)
		return
	}

	// Get provider
	prov, err := providers.For(dbState.Provider)
	if err != nil {
		color.Red("Error: %v", err)
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
	db, err := sql.Open(utils.GetDriverName(dbState.Provider), connStr)
	if err != nil {
		color.Red("Error connecting to database: %v", err)
		return
	}
	defer db.Close()

	ctx := context.Background()

	// Describe table
	var rows *sql.Rows
	if dbState.Provider == utils.ProviderMySQL {
		rows, err = db.QueryContext(ctx, fmt.Sprintf("DESCRIBE %s", providers.QuoteIdentMySQL(tableName)))
	} else {
		rows, err = db.QueryContext(ctx, fmt.Sprintf(`
			SELECT column_name, data_type, is_nullable, column_default
			FROM information_schema.columns
			WHERE table_name = %s
			ORDER BY ordinal_position
		`, providers.QuoteLiteralPG(tableName)))
	}

	if err != nil {
		color.Red("Error describing table: %v", err)
		return
	}
	defer rows.Close()

	color.Cyan("\n📊 Table Structure: %s.%s\n", dbName, tableName)

	var data [][]any
	table := tablewriter.NewTable(os.Stdout)

	if dbState.Provider == utils.ProviderMySQL {
		table.Header([]string{"Field", "Type", "Null", "Key", "Default", "Extra"})

		for rows.Next() {
			var field, typ, null, key, extra string
			var def sql.NullString

			if err := rows.Scan(&field, &typ, &null, &key, &def, &extra); err != nil {
				continue
			}

			defaultVal := "NULL"
			if def.Valid {
				defaultVal = def.String
			}

			data = append(data, []any{field, typ, null, key, defaultVal, extra})
		}

	} else {
		table.Header([]string{"Column", "Type", "Nullable", "Default"})

		for rows.Next() {
			var column, dataType, nullable string
			var def sql.NullString

			if err := rows.Scan(&column, &dataType, &nullable, &def); err != nil {
				continue
			}

			defaultVal := "NULL"
			if def.Valid {
				defaultVal = def.String
			}

			data = append(data, []any{column, dataType, nullable, defaultVal})
		}
	}

	table.Bulk(data)
	table.Render()

	fmt.Println()
}
