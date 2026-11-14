package commands

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/providers"
	"github.com/iqbalmu/dbm/internal/state"
	"github.com/iqbalmu/dbm/pkg/utils"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	execSQL  string
	execFile string
)

var execCmd = &cobra.Command{
	Use:   "exec <database-name>",
	Short: "Execute SQL query",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runExec(args[0])
	},
}

func init() {
	execCmd.Flags().StringVar(&execSQL, "sql", "", "SQL query to execute")
	execCmd.Flags().StringVar(&execFile, "file", "", "SQL file to execute")
}

func runExec(name string) {
	if execSQL == "" && execFile == "" {
		color.Red("Error: Either --sql or --file must be specified")
		return
	}

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

	// Get SQL to execute
	sqlQuery := execSQL
	if execFile != "" {
		content, err := os.ReadFile(execFile)
		if err != nil {
			color.Red("Error reading file: %v", err)
			return
		}
		sqlQuery = string(content)
	}

	// Execute query
	rows, err := db.QueryContext(ctx, sqlQuery)
	if err != nil {
		// Try as exec (for INSERT, UPDATE, DELETE, etc.)
		result, execErr := db.ExecContext(ctx, sqlQuery)
		if execErr != nil {
			color.Red("Error executing query: %v", err)
			return
		}

		affected, _ := result.RowsAffected()
		color.Green("\n✓ Query executed successfully")
		fmt.Printf("  Rows affected: %d\n\n", affected)
		return
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		color.Red("Error getting columns: %v", err)
		return
	}

	// Read rows into data
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	var data [][]any
	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make([]any, len(columns))
		for i, val := range values {
			if val == nil {
				row[i] = "NULL"
			} else {
				row[i] = val
			}
		}
		data = append(data, row)
	}

	fmt.Println()

	table := tablewriter.NewTable(os.Stdout)
	table.Header(columns)
	table.Bulk(data)
	table.Render()

	color.Green("\n%d rows returned\n", len(data))
}
