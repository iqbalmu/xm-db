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
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogs(args[0])
	},
}

func init() {
	logsCmd.Flags().StringVarP(&logsTail, "tail", "n", "100", "Number of lines to show from the end of the logs")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
}

func runLogs(name string) error {
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

	color.Cyan("Logs for '%s':\n", name)
	fmt.Println(color.CyanString("════════════════════════════════════════"))

	logs, err := dockerMgr.GetContainerLogs(ctx, db.ContainerID, db.ContainerName, logsTail, logsFollow)
	if err != nil {
		return fmt.Errorf("retrieving logs: %w", err)
	}
	defer logs.Close()

	// Container streams are multiplexed (stdout/stderr frames); decode them,
	// otherwise the raw frame headers corrupt the output.
	if _, err := stdcopy.StdCopy(color.Output, os.Stderr, logs); err != nil {
		return fmt.Errorf("reading logs: %w", err)
	}

	if !logsFollow {
		fmt.Println()
	}
	return nil
}
