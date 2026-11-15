package commands

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/iqbalmu/xm-db/pkg/utils"
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
	execCmd.Flags().StringVar(&execSQL, "sql", "", "SQL statement to execute")
	execCmd.Flags().StringVar(&execFile, "file", "", "SQL script file to execute")
}

func runExec(name string) {
	if (execSQL == "") == (execFile == "") {
		color.Red("Error: specify exactly one of --sql or --file")
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

	if execFile != "" {
		// Script files carry many statements; they run through the engine's
		// own client inside the container, which supports them everywhere.
		runExecFile(dbState)
		return
	}

	runExecSQL(dbState)
}

// runExecFile streams a script through the database client inside the
// container, so multi-statement files work on both providers and dumps of any
// size pass through without buffering.
func runExecFile(dbState *state.Database) {
	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	ctx := context.Background()

	running, err := dockerMgr.IsContainerRunning(ctx, dbState.ContainerID, dbState.ContainerName)
	if err != nil || !running {
		color.Red("Error: Database '%s' is not running", dbState.Name)
		return
	}

	sqlFile, err := os.Open(execFile)
	if err != nil {
		color.Red("Error opening file: %v", err)
		return
	}
	defer sqlFile.Close()

	var cmd []string
	if dbState.Provider == utils.ProviderMySQL {
		cmd = []string{"mysql", "-uroot", "-p" + dbState.RootPassword, dbState.DatabaseName}
	} else {
		cmd = []string{"psql", "-U", "postgres", "-d", dbState.DatabaseName, "-v", "ON_ERROR_STOP=1"}
	}

	color.Cyan("Running %s on '%s'...\n", execFile, dbState.Name)

	res, err := dockerMgr.Exec(ctx, dbState.ContainerID, dbState.ContainerName, cmd, sqlFile, os.Stdout)
	if err != nil {
		color.Red("Error executing file: %v", err)
		return
	}
	if res.ExitCode != 0 {
		color.Red("Error executing %s (exit %d):\n%s", execFile, res.ExitCode, res.Stderr)
		return
	}
	if res.Stderr != "" {
		color.Yellow("Warnings:\n%s", res.Stderr)
	}
}

// runExecSQL executes a single statement over a direct connection, printing
// result rows as a table or the affected row count.
func runExecSQL(dbState *state.Database) {
	prov, err := providers.For(dbState.Provider)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	db, err := prov.Connect(&providers.DBConfig{
		Host:         "localhost",
		Port:         dbState.Port,
		RootPassword: dbState.RootPassword,
		DatabaseName: dbState.DatabaseName,
	})
	if err != nil {
		color.Red("Error connecting to database: %v", err)
		return
	}
	defer db.Close()

	ctx := context.Background()

	if isRowQuery(execSQL) {
		rows, err := db.QueryContext(ctx, execSQL)
		if err != nil {
			// Detection is a guess; let the exec path try before failing.
			if res, execErr := db.ExecContext(ctx, execSQL); execErr == nil {
				printExecResult(res)
				return
			}
			color.Red("Error executing query: %v", err)
			return
		}
		defer rows.Close()
		printQueryRows(rows)
		return
	}

	res, err := db.ExecContext(ctx, execSQL)
	if err != nil {
		// Some row-returning statements slip past detection (e.g. CALL);
		// fall back to the query path so their rows still print.
		if rows, queryErr := db.QueryContext(ctx, execSQL); queryErr == nil {
			defer rows.Close()
			printQueryRows(rows)
			return
		}
		color.Red("Error executing statement: %v", err)
		return
	}
	printExecResult(res)
}

// isRowQuery guesses whether a statement returns rows from its first
// keyword. DDL and DML fall through to Exec so affected rows are reported.
func isRowQuery(stmt string) bool {
	s := strings.TrimSpace(stmt)
	for {
		switch {
		case strings.HasPrefix(s, "--"):
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = strings.TrimSpace(s[i+1:])
				continue
			}
			return false
		case strings.HasPrefix(s, "/*"):
			if i := strings.Index(s, "*/"); i >= 0 {
				s = strings.TrimSpace(s[i+2:])
				continue
			}
			return false
		}
		break
	}
	if s == "" {
		return false
	}

	fields := strings.Fields(s)
	switch strings.ToUpper(fields[0]) {
	case "SELECT", "WITH", "SHOW", "EXPLAIN", "DESCRIBE", "DESC", "TABLE", "VALUES":
		return true
	}
	return false
}

func printQueryRows(rows *sql.Rows) {
	columns, err := rows.Columns()
	if err != nil {
		color.Red("Error getting columns: %v", err)
		return
	}

	values := make([]any, len(columns))
	valuePtrs := make([]any, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	var data [][]any
	var scanErr error
	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			if scanErr == nil {
				scanErr = err
			}
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
	if scanErr != nil {
		color.Yellow("Warning: some rows could not be read: %v", scanErr)
	}
	if err := rows.Err(); err != nil {
		color.Yellow("Warning: iteration stopped early: %v", err)
	}

	fmt.Println()

	table := tablewriter.NewTable(os.Stdout)
	table.Header(columns)
	table.Bulk(data)
	table.Render()

	color.Green("\n%d rows returned\n", len(data))
}

func printExecResult(res sql.Result) {
	color.Green("\nStatement executed successfully")

	if affected, err := res.RowsAffected(); err == nil {
		fmt.Printf("  Rows affected: %d\n\n", affected)
		return
	}
	fmt.Println()
}
