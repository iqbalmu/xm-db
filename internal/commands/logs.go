package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/state"
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

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	ctx := context.Background()

	color.Cyan("Logs for '%s':\n", name)
	fmt.Println(color.CyanString("════════════════════════════════════════"))

	logs, err := dockerMgr.GetContainerLogs(ctx, db.ContainerID, db.ContainerName, logsTail, logsFollow)
	if err != nil {
		color.Red("Error retrieving logs: %v", err)
		return
	}
	defer logs.Close()

	// Container streams are multiplexed (stdout/stderr frames); decode them,
	// otherwise the raw frame headers corrupt the output.
	if _, err := stdcopy.StdCopy(color.Output, os.Stderr, logs); err != nil {
		color.Red("Error reading logs: %v", err)
		return
	}

	if !logsFollow {
		fmt.Println()
	}
}
