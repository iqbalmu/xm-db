package commands

import (
	"context"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/spf13/cobra"
)

var (
	removeForce    bool
	removeKeepData bool
)

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
	removeCmd.Flags().BoolVar(&removeKeepData, "keep-data", false, "Keep the data volume (reused if the database is created again)")
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

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	ctx := context.Background()

	color.Cyan("Removing database '%s'...", name)

	running, _ := dockerMgr.IsContainerRunning(ctx, db.ContainerID, db.ContainerName)
	if running {
		color.White("  Stopping container...")
		dockerMgr.StopContainer(ctx, db.ContainerID, db.ContainerName)
	}

	color.White("  Removing container...")
	if err := dockerMgr.RemoveContainer(ctx, db.ContainerID, db.ContainerName); err != nil {
		color.Yellow("  Warning: Could not remove container: %v", err)
	}

	// Named volumes survive container removal (RemoveVolumes only deletes
	// anonymous ones), so delete the data volume explicitly.
	if db.VolumeName != "" {
		if removeKeepData {
			color.White("  Keeping data volume '%s' (--keep-data)", db.VolumeName)
		} else {
			color.White("  Removing data volume '%s'...", db.VolumeName)
			if err := dockerMgr.RemoveVolume(ctx, db.VolumeName); err != nil {
				color.Yellow("  Warning: Could not remove data volume: %v", err)
			}
		}
	}

	if err := stateMgr.Delete(name); err != nil {
		color.Red("Error removing state: %v", err)
		return
	}

	color.Green("Database '%s' removed successfully", name)
}
