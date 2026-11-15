package commands

import (
	"context"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop a database",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runStop(args[0])
	},
}

func runStop(name string) {
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
	if err == nil && !running {
		color.Yellow("Database '%s' is already stopped", name)
		return
	}

	color.Cyan("⏸️  Stopping database '%s'...", name)

	if err := dockerMgr.StopContainer(ctx, db.ContainerID, db.ContainerName); err != nil {
		color.Red("Error stopping container: %v", err)
		return
	}

	color.Green("✓ Database '%s' stopped successfully", name)
}
