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

	if backupOutput == "" {
		timestamp := time.Now().Format("20060102_150405")
		backupOutput = fmt.Sprintf("backup_%s_%s.sql", name, timestamp)
	}

	absPath, err := filepath.Abs(backupOutput)
	if err != nil {
		color.Red("Error: Invalid output path: %v", err)
		return
	}

	color.Cyan("\nCreating backup for '%s'...\n", name)

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

	// Owner-only file: the dump contains database data and is streamed
	// straight to disk, never buffered whole in memory.
	dumpFile, err := os.OpenFile(absPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		color.Red("Error creating backup file: %v", err)
		return
	}

	res, err := dockerMgr.Exec(ctx, db.ContainerID, db.ContainerName, cmd, nil, dumpFile)
	dumpFile.Close()
	if err != nil {
		os.Remove(absPath) // a partial dump is worse than none
		color.Red("Error creating backup: %v", err)
		return
	}
	if res.ExitCode != 0 {
		os.Remove(absPath)
		color.Red("Error creating backup (exit %d):\n%s", res.ExitCode, res.Stderr)
		return
	}
	if res.Stderr != "" {
		color.Yellow("Warnings:\n%s", res.Stderr)
	}

	fileInfo, _ := os.Stat(absPath)
	size := float64(fileInfo.Size()) / 1024 / 1024 // MB

	color.Green("\nBackup created successfully!\n")
	color.Cyan("  File: %s", absPath)
	color.Cyan("  Size: %.2f MB", size)
	fmt.Println()
}
