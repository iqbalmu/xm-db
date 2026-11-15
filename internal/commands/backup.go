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
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackup(args[0])
	},
}

func init() {
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "Output file path (default: ./backup_<name>_<timestamp>.sql)")
}

func runBackup(name string) error {
	color.Cyan("\nCreating backup for '%s'...\n", name)

	absPath, sizeBytes, stderr, err := backupDatabase(context.Background(), name, backupOutput)
	if err != nil {
		return err
	}
	if stderr != "" {
		color.Yellow("Warnings:\n%s", stderr)
	}

	size := float64(sizeBytes) / 1024 / 1024 // MB
	color.Green("\nBackup created successfully!\n")
	color.Cyan("  File: %s", absPath)
	color.Cyan("  Size: %.2f MB", size)
	fmt.Println()
	return nil
}

// backupDatabase dumps the named database to outputPath (or a timestamped
// default in the current working directory if empty) without writing to stdout.
func backupDatabase(ctx context.Context, name, outputPath string) (string, int64, string, error) {
	stateMgr, err := state.NewManager()
	if err != nil {
		return "", 0, "", err
	}

	db, err := stateMgr.Load(name)
	if err != nil {
		return "", 0, "", fmt.Errorf("database '%s' not found", name)
	}

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		return "", 0, "", err
	}

	running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID, db.ContainerName)
	if err != nil || !running {
		return "", 0, "", fmt.Errorf("database '%s' is not running", name)
	}

	if outputPath == "" {
		timestamp := time.Now().Format("20060102_150405")
		outputPath = fmt.Sprintf("backup_%s_%s.sql", name, timestamp)
	}

	absPath, err := filepath.Abs(outputPath)
	if err != nil {
		return "", 0, "", fmt.Errorf("invalid output path: %w", err)
	}

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
		return "", 0, "", fmt.Errorf("creating backup file: %w", err)
	}

	res, err := dockerMgr.Exec(ctx, db.ContainerID, db.ContainerName, cmd, nil, dumpFile)
	dumpFile.Close()
	if err != nil {
		os.Remove(absPath) // a partial dump is worse than none
		return "", 0, "", fmt.Errorf("creating backup: %w", err)
	}
	if res.ExitCode != 0 {
		os.Remove(absPath)
		return "", 0, "", fmt.Errorf("creating backup (exit %d):\n%s", res.ExitCode, res.Stderr)
	}

	fileInfo, _ := os.Stat(absPath)
	var sizeBytes int64
	if fileInfo != nil {
		sizeBytes = fileInfo.Size()
	}
	return absPath, sizeBytes, res.Stderr, nil
}
