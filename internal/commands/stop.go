package commands

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/spf13/cobra"
)

var stopGroup string

var stopCmd = &cobra.Command{
	Use:   "stop [name]",
	Short: "Stop a database (or all databases in a group with -g)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if stopGroup != "" {
			if len(args) > 0 {
				return fmt.Errorf("cannot specify both a database name and --group")
			}
			return runStopGroup(stopGroup)
		}
		if len(args) == 0 {
			return fmt.Errorf("specify a database name or --group <group>")
		}
		return runStop(args[0])
	},
}

func init() {
	stopCmd.Flags().StringVarP(&stopGroup, "group", "g", "", "Stop all databases in a group ('global' for ungrouped)")
}

func runStopGroup(group string) error {
	stateMgr, err := state.NewManager()
	if err != nil {
		return err
	}

	dbs, err := stateMgr.List()
	if err != nil {
		return fmt.Errorf("loading databases: %w", err)
	}
	dbs = filterByGroup(dbs, group)
	if len(dbs) == 0 {
		return fmt.Errorf("no databases found in group '%s'", group)
	}

	for _, db := range dbs {
		if err := runStop(db.Name); err != nil {
			return err
		}
	}
	return nil
}

func runStop(name string) error {
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

	running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID, db.ContainerName)
	if err == nil && !running {
		color.Yellow("Database '%s' is already stopped", name)
		return nil
	}

	color.Cyan("Stopping database '%s'...", name)

	if err := dockerMgr.StopContainer(ctx, db.ContainerID, db.ContainerName); err != nil {
		return fmt.Errorf("stopping container: %w", err)
	}

	color.Green("Database '%s' stopped successfully", name)
	return nil
}
