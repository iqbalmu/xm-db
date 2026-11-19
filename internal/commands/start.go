package commands

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/xmdb/internal/container"
	"github.com/iqbalmu/xmdb/internal/state"
	"github.com/spf13/cobra"
)

var startGroup string

var startCmd = &cobra.Command{
	Use:   "start [name]",
	Short: "Start a database (or all databases in a group with -g)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if startGroup != "" {
			if len(args) > 0 {
				return fmt.Errorf("cannot specify both a database name and --group")
			}
			return runStartGroup(startGroup)
		}
		if len(args) == 0 {
			return fmt.Errorf("specify a database name or --group <group>")
		}
		return runStart(args[0])
	},
}

func init() {
	startCmd.Flags().StringVarP(&startGroup, "group", "g", "", "Start all databases in a group ('global' for ungrouped)")
}

func runStartGroup(group string) error {
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
		if err := runStart(db.Name); err != nil {
			return err
		}
	}
	return nil
}

func runStart(name string) error {
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
	if err != nil {
		return fmt.Errorf("checking container status: %w", err)
	}

	if running {
		color.Yellow("Database '%s' is already running", name)
		return nil
	}

	color.Cyan("Starting database '%s'...", name)

	if err := dockerMgr.StartContainer(ctx, db.ContainerID, db.ContainerName); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	color.Green("Database '%s' started successfully", name)
	return nil
}
