package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/iqbalmu/xm-db/pkg/utils"
	"github.com/spf13/cobra"
)

var restoreForce bool

var restoreCmd = &cobra.Command{
	Use:   "restore <name> <backup-file>",
	Short: "Restore database from SQL file",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runRestore(args[0], args[1])
	},
}

func init() {
	restoreCmd.Flags().BoolVarP(&restoreForce, "force", "f", false, "Skip confirmation prompt")
}

func runRestore(name, backupFile string) {
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

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	ctx := context.Background()

	running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID, db.ContainerName)
	if err != nil || !running {
		color.Red("Error: Database '%s' is not running", name)
		return
	}

	absPath, err := filepath.Abs(backupFile)
	if err != nil {
		color.Red("Error: Invalid backup file path: %v", err)
		return
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		color.Red("Error: Backup file not found: %s", absPath)
		return
	}

	if !restoreForce {
		var confirm bool
		prompt := &survey.Confirm{
			Message: color.YellowString("⚠️  This will replace all data in database '%s'. Continue?", name),
			Default: false,
		}
		if err := survey.AskOne(prompt, &confirm); err != nil {
			return
		}
		if !confirm {
			color.Yellow("Cancelled")
			return
		}
	}

	// Stream the SQL file through stdin instead of reading it whole: dumps
	// can far exceed available memory.
	sqlFile, err := os.Open(absPath)
	if err != nil {
		color.Red("Error reading backup file: %v", err)
		return
	}
	defer sqlFile.Close()

	color.Cyan("\n📥 Restoring database '%s' from backup...\n", name)

	var cmd []string
	if db.Provider == utils.ProviderMySQL {
		cmd = []string{
			"mysql",
			"-u", "root",
			fmt.Sprintf("-p%s", db.RootPassword),
			db.DatabaseName,
		}
	} else {
		cmd = []string{
			"psql",
			"-U", "postgres",
			"-d", db.DatabaseName,
			"-v", "ON_ERROR_STOP=1",
		}
	}

	// Pipe the SQL file through stdin: passing it as an argument would hit
	// exec argument size limits on large dumps.
	res, err := dockerMgr.Exec(ctx, db.ContainerID, db.ContainerName, cmd, sqlFile, os.Stdout)
	if err != nil {
		color.Red("Error restoring database: %v", err)
		return
	}
	if res.ExitCode != 0 {
		color.Red("Restore failed (exit %d):\n%s", res.ExitCode, res.Stderr)
		return
	}
	if res.Stderr != "" {
		color.Yellow("Warnings:\n%s", res.Stderr)
	}

	fileInfo, _ := os.Stat(absPath)
	size := float64(fileInfo.Size()) / 1024 / 1024 // MB

	color.Green("\n✓ Database restored successfully!\n")
	color.Cyan("  File: %s", absPath)
	color.Cyan("  Size: %.2f MB", size)
	fmt.Println()
}
