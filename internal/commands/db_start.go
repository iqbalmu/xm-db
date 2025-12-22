package commands

import (
	"context"

	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/container"
	"github.com/iqbalmu/dbm/internal/state"
	"github.com/spf13/cobra"
)

var dbStartCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a database",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runDBStart(args[0])
	},
}

func runDBStart(name string) {
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

	// Check if already running
	running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID)
	if err != nil {
		color.Red("Error checking container status: %v", err)
		return
	}

	if running {
		color.Yellow("Database '%s' is already running", name)
		return
	}

	color.Cyan("▶️  Starting database '%s'...", name)

	if err := dockerMgr.StartContainer(ctx, db.ContainerID); err != nil {
		color.Red("Error starting container: %v", err)
		return
	}

	color.Green("✓ Database '%s' started successfully", name)
}
