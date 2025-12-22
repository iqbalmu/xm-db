package db

import (
	"context"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/container"
	"github.com/iqbalmu/dbm/internal/state"
	"github.com/spf13/cobra"
)

var removeForce bool

var removeCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a database",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runRemove(args[0])
	},
}

func init() {
	removeCmd.Flags().BoolVarP(&removeForce, "force", "f", false, "Force removal without confirmation")
}

func runRemove(name string) {
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

	// Confirmation
	if !removeForce {
		var confirm bool
		prompt := &survey.Confirm{
			Message: color.RedString("Are you sure you want to remove database '%s'? This will delete all data.", name),
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

	dockerMgr, err := container.NewDockerManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	ctx := context.Background()

	color.Cyan("🗑️  Removing database '%s'...", name)

	// Stop container if running
	running, _ := dockerMgr.IsContainerRunning(ctx, db.ContainerID)
	if running {
		color.White("  Stopping container...")
		dockerMgr.StopContainer(ctx, db.ContainerID)
	}

	// Remove container
	color.White("  Removing container...")
	if err := dockerMgr.RemoveContainer(ctx, db.ContainerID); err != nil {
		color.Yellow("  Warning: Could not remove container: %v", err)
	}

	// Remove state
	if err := stateMgr.Delete(name); err != nil {
		color.Red("Error removing state: %v", err)
		return
	}

	color.Green("✓ Database '%s' removed successfully", name)
}
