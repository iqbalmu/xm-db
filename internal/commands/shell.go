package commands

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/iqbalmu/xm-db/pkg/utils"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:     "shell <database-name>",
	Aliases: []string{"sh", "console"},
	Short:   "Open an interactive SQL shell (psql/mysql) inside the container",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runShell(args[0])
	},
}

func runShell(name string) error {
	stateMgr, err := state.NewManager()
	if err != nil {
		return err
	}

	dbState, err := stateMgr.Load(name)
	if err != nil {
		return fmt.Errorf("database '%s' not found", name)
	}

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		return err
	}

	ctx := context.Background()
	running, err := dockerMgr.IsContainerRunning(ctx, dbState.ContainerID, dbState.ContainerName)
	if err != nil || !running {
		return fmt.Errorf("database '%s' is not running (start it with: xmdb start %s)", name, name)
	}

	var cmd []string
	if dbState.Provider == utils.ProviderMySQL {
		cmd = []string{"mysql", "-uroot", "-p" + dbState.RootPassword, dbState.DatabaseName}
	} else {
		cmd = []string{"psql", "-U", "postgres", "-d", dbState.DatabaseName}
	}

	color.Cyan("Opening %s shell for '%s' (type \\q or exit to quit)...\n", dbState.Provider, name)
	return dockerMgr.ExecInteractive(ctx, dbState.ContainerID, dbState.ContainerName, cmd)
}
