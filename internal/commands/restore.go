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
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRestore(args[0], args[1])
	},
}

func init() {
	restoreCmd.Flags().BoolVarP(&restoreForce, "force", "f", false, "Skip confirmation prompt")
}

func runRestore(name, backupFile string) error {
	stateMgr, err := state.NewManager()
	if err != nil {
		return err
	}

	db, err := stateMgr.Load(name)
	if err != nil {
		return fmt.Errorf("database '%s' not found", name)
	}

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		return err
	}

	ctx := context.Background()

	running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID, db.ContainerName)
	if err != nil || !running {
		return fmt.Errorf("database '%s' is not running", name)
	}

	absPath, err := filepath.Abs(backupFile)
	if err != nil {
		return fmt.Errorf("invalid backup file path: %w", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", absPath)
	}

	if !restoreForce {
		var confirm bool
		prompt := &survey.Confirm{
			Message: color.YellowString("WARNING: This will replace all data in database '%s'. Continue?", name),
			Default: false,
		}
		if err := survey.AskOne(prompt, &confirm); err != nil {
			return err
		}
		if !confirm {
			color.Yellow("Cancelled")
			return nil
		}
	}

	// Stream the SQL file through stdin instead of reading it whole: dumps
	// can far exceed available memory.
	sqlFile, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("reading backup file: %w", err)
	}
	defer sqlFile.Close()

	color.Cyan("\nRestoring database '%s' from backup...\n", name)

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
		return fmt.Errorf("restoring database: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("restore failed (exit %d):\n%s", res.ExitCode, res.Stderr)
	}
	if res.Stderr != "" {
		color.Yellow("Warnings:\n%s", res.Stderr)
	}

	fileInfo, _ := os.Stat(absPath)
	size := float64(fileInfo.Size()) / 1024 / 1024 // MB

	color.Green("\nDatabase restored successfully!\n")
	color.Cyan("  File: %s", absPath)
	color.Cyan("  Size: %.2f MB", size)
	fmt.Println()
	return nil
}
