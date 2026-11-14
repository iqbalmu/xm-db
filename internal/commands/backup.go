package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/iqbalmu/xm-db/pkg/utils"
	"github.com/spf13/cobra"
)

var backupOutput string

var backupCmd = &cobra.Command{
	Use:   "backup <name>",
	Short: "Backup database to SQL file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runBackup(args[0])
	},
}

func init() {
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "Output file path (default: ./backup_<name>_<timestamp>.sql)")
}

func runBackup(name string) {
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

	dockerMgr, err := container.NewDockerManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	ctx := context.Background()

	// Check if container is running
	running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID)
	if err != nil || !running {
		color.Red("Error: Database '%s' is not running", name)
		return
	}

	// Generate output filename if not provided
	if backupOutput == "" {
		timestamp := time.Now().Format("20060102_150405")
		backupOutput = fmt.Sprintf("backup_%s_%s.sql", name, timestamp)
	}

	// Make path absolute
	absPath, err := filepath.Abs(backupOutput)
	if err != nil {
		color.Red("Error: Invalid output path: %v", err)
		return
	}

	color.Cyan("\n💾 Creating backup for '%s'...\n", name)

	var cmd []string
	if db.Provider == utils.ProviderMySQL {
		cmd = []string{
			"mysqldump",
			"-u", "root",
			fmt.Sprintf("-p%s", db.RootPassword),
			db.DatabaseName,
		}
	} else {
		cmd = []string{
			"pg_dump",
			"-U", "postgres",
			db.DatabaseName,
		}
	}

	res, err := dockerMgr.Exec(ctx, db.ContainerID, cmd, "")
	if err != nil {
		color.Red("Error creating backup: %v", err)
		return
	}
	if res.ExitCode != 0 {
		color.Red("Error creating backup (exit %d):\n%s", res.ExitCode, res.Stderr)
		return
	}
	if res.Stderr != "" {
		color.Yellow("Warnings:\n%s", res.Stderr)
	}

	// Write to file — owner-only, the dump contains database data.
	if err := os.WriteFile(absPath, []byte(res.Stdout), 0o600); err != nil {
		color.Red("Error writing backup file: %v", err)
		return
	}

	// Get file size
	fileInfo, _ := os.Stat(absPath)
	size := float64(fileInfo.Size()) / 1024 / 1024 // MB

	color.Green("\n✓ Backup created successfully!\n")
	color.Cyan("  File: %s", absPath)
	color.Cyan("  Size: %.2f MB", size)
	fmt.Println()
}
