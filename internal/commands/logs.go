package commands

import (
	"context"
	"fmt"
	"io"

	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/container"
	"github.com/iqbalmu/dbm/internal/state"
	"github.com/spf13/cobra"
)

var (
	logsTail   string
	logsFollow bool
)

var logsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "View database container logs",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runLogs(args[0])
	},
}

func init() {
	logsCmd.Flags().StringVarP(&logsTail, "tail", "n", "100", "Number of lines to show from the end of the logs")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
}

func runLogs(name string) {
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

	color.Cyan("📋 Logs for '%s':\n", name)
	fmt.Println(color.CyanString("════════════════════════════════════════"))

	logs, err := dockerMgr.GetContainerLogs(ctx, db.ContainerID, logsTail, logsFollow)
	if err != nil {
		color.Red("Error retrieving logs: %v", err)
		return
	}
	defer logs.Close()

	// Copy logs to stdout
	if _, err := io.Copy(color.Output, logs); err != nil {
		color.Red("Error reading logs: %v", err)
		return
	}

	if !logsFollow {
		fmt.Println()
	}
}
