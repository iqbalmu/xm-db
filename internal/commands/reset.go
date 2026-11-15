package commands

import (
	"context"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/spf13/cobra"
)

var resetForce bool

var resetCmd = &cobra.Command{
	Use:   "reset <database-name>",
	Short: "Wipe all data and re-initialize a database with the same port and credentials",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReset(args[0])
	},
}

func init() {
	resetCmd.Flags().BoolVarP(&resetForce, "force", "f", false, "Skip confirmation prompt")
}

func runReset(name string) error {
	stateMgr, err := state.NewManager()
	if err != nil {
		return err
	}

	dbState, err := stateMgr.Load(name)
	if err != nil {
		return fmt.Errorf("database '%s' not found", name)
	}

	prov, err := providers.For(dbState.Provider)
	if err != nil {
		return err
	}

	if !resetForce {
		var confirm bool
		prompt := &survey.Confirm{
			Message: color.RedString("WARNING: This will wipe all data in '%s' and recreate it fresh. Continue?", name),
			Default: false,
		}
		if err := survey.AskOne(prompt, &confirm); err != nil {
			return err
		}
		if !confirm {
			color.Yellow("Cancelled")
			return nil
		}
	}

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		return err
	}

	ctx := context.Background()
	color.Cyan("\nResetting database '%s'...\n", name)

	if running, _ := dockerMgr.IsContainerRunning(ctx, dbState.ContainerID, dbState.ContainerName); running {
		color.White("  Stopping container...")
		_ = dockerMgr.StopContainer(ctx, dbState.ContainerID, dbState.ContainerName)
	}

	color.White("  Removing old container...")
	_ = dockerMgr.RemoveContainer(ctx, dbState.ContainerID, dbState.ContainerName)

	volumePath := ""
	if dbState.VolumeName != "" {
		volumePath = prov.GetVolumePath(dbState.Version)
		color.White("  Removing data volume '%s'...", dbState.VolumeName)
		if err := dockerMgr.RemoveVolume(ctx, dbState.VolumeName); err != nil {
			color.Yellow("  Warning: could not remove volume: %v", err)
		}
	}

	dbConfig := &providers.DBConfig{
		Host:         "127.0.0.1",
		Port:         dbState.Port,
		RootPassword: dbState.RootPassword,
		DatabaseName: dbState.DatabaseName,
		User:         dbState.User,
		UserPassword: dbState.UserPassword,
	}

	containerConfig := &container.ContainerConfig{
		Name:          dbState.ContainerName,
		Image:         prov.GetImageName(dbState.Version),
		Port:          dbState.Port,
		ContainerPort: prov.GetContainerPort(),
		Env:           prov.GetEnvVars(dbConfig),
		VolumeName:    dbState.VolumeName,
		VolumePath:    volumePath,
	}

	color.White("  Creating fresh container...")
	containerID, err := dockerMgr.CreateContainer(ctx, containerConfig)
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	color.White("  Starting container...")
	if err := dockerMgr.StartContainer(ctx, containerID, dbState.ContainerName); err != nil {
		cleanup(dockerMgr, ctx, containerID, dbState.ContainerName, dbState.VolumeName)
		return fmt.Errorf("starting container: %w", err)
	}

	adminConfig := &providers.DBConfig{
		Host:         "127.0.0.1",
		Port:         dbState.Port,
		RootPassword: dbState.RootPassword,
		DatabaseName: dbState.DatabaseName,
	}

	color.White("  Waiting for database to accept connections...")
	if err := waitForDatabase(ctx, dockerMgr, containerID, dbState.ContainerName, prov, adminConfig, containerStartTimeout); err != nil {
		cleanup(dockerMgr, ctx, containerID, dbState.ContainerName, dbState.VolumeName)
		return err
	}

	if dbState.User != "" {
		color.White("  Provisioning user '%s'...", dbState.User)
		db, err := prov.Connect(adminConfig)
		if err != nil {
			cleanup(dockerMgr, ctx, containerID, dbState.ContainerName, dbState.VolumeName)
			return fmt.Errorf("connecting to database: %w", err)
		}
		if err := prov.CreateUser(ctx, db, dbState.User, dbState.UserPassword, dbState.DatabaseName); err != nil {
			db.Close()
			cleanup(dockerMgr, ctx, containerID, dbState.ContainerName, dbState.VolumeName)
			return fmt.Errorf("creating user: %w", err)
		}
		db.Close()
	}

	dbState.ContainerID = containerID
	if err := stateMgr.Save(dbState); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	color.Green("\nDatabase '%s' reset successfully! (Port %d, credentials unchanged)\n", name, dbState.Port)
	return nil
}
