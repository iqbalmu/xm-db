package commands

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/iqbalmu/xmdb/internal/container"
	"github.com/iqbalmu/xmdb/internal/providers"
	"github.com/iqbalmu/xmdb/internal/state"
	"github.com/iqbalmu/xmdb/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	clonePort       int
	cloneGroup      string
	cloneVersion    string
	cloneWithVolume bool
)

var cloneCmd = &cobra.Command{
	Use:   "clone <source> <target>",
	Short: "Clone an existing database and its data into a new instance",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runClone(cmd, args[0], args[1])
	},
}

func init() {
	cloneCmd.Flags().IntVar(&clonePort, "port", 0, "Host port for cloned database (default: next free port)")
	cloneCmd.Flags().StringVarP(&cloneGroup, "group", "g", "", "Group for cloned database (default: inherit from source)")
	cloneCmd.Flags().StringVarP(&cloneVersion, "version", "v", "", "Database version override (default: inherit from source)")
	cloneCmd.Flags().BoolVar(&cloneWithVolume, "volume", true, "Create with persistent volume (default: true)")
}

type cloneOptions struct {
	SourceName string
	TargetName string
	Port       int
	Group      string
	GroupSet   bool
	Version    string
	WithVolume bool
}

func runClone(cmd *cobra.Command, sourceName, targetName string) error {
	stateMgr, err := state.NewManager()
	if err != nil {
		return err
	}

	opts := cloneOptions{
		SourceName: sourceName,
		TargetName: targetName,
		Port:       clonePort,
		Group:      strings.TrimSpace(cloneGroup),
		GroupSet:   cmd.Flags().Changed("group"),
		Version:    strings.TrimSpace(cloneVersion),
		WithVolume: cloneWithVolume,
	}

	color.Cyan("\nCloning database '%s' -> '%s'...\n", sourceName, targetName)
	dbState, dbConfig, err := cloneDatabase(context.Background(), stateMgr, opts, func(msg string) {
		color.White(msg)
	})
	if err != nil {
		return err
	}

	prov, _ := providers.For(dbState.Provider)
	color.Green("\nDatabase '%s' cloned successfully from '%s'!\n", dbState.Name, sourceName)
	fmt.Println("Connection Information:")
	color.Cyan("  Provider: %s", dbState.Provider)
	color.Cyan("  Port: %d", dbState.Port)
	color.Cyan("  Database: %s", dbState.DatabaseName)
	if dbState.User != "" {
		color.Cyan("  User: %s", dbState.User)
		color.Cyan("  Password: %s", dbState.UserPassword)
		fmt.Println()
		color.White("Connection String:")
		fmt.Printf("  %s\n", prov.GetConnectionString(dbConfig))
	}
	fmt.Println()
	return nil
}

// cloneDatabase duplicates a managed database (schema and data) into a new
// managed instance via logical dump/restore streaming. If source is stopped,
// it is temporarily started for the dump and stopped again afterwards.
func cloneDatabase(ctx context.Context, stateMgr *state.Manager, opts cloneOptions, onProgress func(string)) (*state.Database, *providers.DBConfig, error) {
	report := func(msg string) {
		if onProgress != nil {
			onProgress(msg)
		}
	}

	src, err := stateMgr.Load(opts.SourceName)
	if err != nil {
		return nil, nil, fmt.Errorf("source database '%s' not found", opts.SourceName)
	}

	if err := dbNameValidator(stateMgr)(opts.TargetName); err != nil {
		return nil, nil, err
	}

	prov, err := providers.For(src.Provider)
	if err != nil {
		return nil, nil, err
	}

	version := src.Version
	if opts.Version != "" {
		if err := versionValidator(opts.Version); err != nil {
			return nil, nil, err
		}
		version = opts.Version
	}

	group := src.Group
	if opts.GroupSet {
		if opts.Group != "" {
			if err := groupValidator(opts.Group); err != nil {
				return nil, nil, err
			}
		}
		group = strings.TrimSpace(opts.Group)
	}

	port := opts.Port
	if port == 0 {
		port = nextFreePort(stateMgr, prov.GetDefaultPort())
	}

	rootPassword, err := utils.GeneratePassword(utils.DefaultPasswordLength)
	if err != nil {
		return nil, nil, fmt.Errorf("generating password: %w", err)
	}

	createUser := src.User != ""
	var userPassword string
	if createUser {
		userPassword, err = utils.GeneratePassword(utils.DefaultPasswordLength)
		if err != nil {
			return nil, nil, fmt.Errorf("generating password: %w", err)
		}
	}

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		return nil, nil, err
	}

	wasRunning, err := dockerMgr.IsContainerRunning(ctx, src.ContainerID, src.ContainerName)
	if err != nil {
		return nil, nil, fmt.Errorf("checking source container status: %w", err)
	}

	if !wasRunning {
		report(fmt.Sprintf("Starting source database '%s' temporarily...", src.Name))
		if err := dockerMgr.StartContainer(ctx, src.ContainerID, src.ContainerName); err != nil {
			return nil, nil, fmt.Errorf("starting source database '%s': %w", src.Name, err)
		}
		defer func() {
			report(fmt.Sprintf("Stopping source database '%s'...", src.Name))
			_ = dockerMgr.StopContainer(ctx, src.ContainerID, src.ContainerName)
		}()

		srcAdminCfg := &providers.DBConfig{
			Host:         "127.0.0.1",
			Port:         src.Port,
			RootPassword: src.RootPassword,
			DatabaseName: src.DatabaseName,
		}
		if err := waitForDatabase(ctx, dockerMgr, src.ContainerID, src.ContainerName, prov, srcAdminCfg, containerStartTimeout); err != nil {
			return nil, nil, fmt.Errorf("waiting for source database '%s': %w", src.Name, err)
		}
	}

	targetOpts := createOptions{
		Name:         opts.TargetName,
		Provider:     src.Provider,
		Version:      version,
		Port:         port,
		Group:        group,
		RootPassword: rootPassword,
		CreateUser:   createUser,
		Username:     src.User,
		UserPassword: userPassword,
		WithVolume:   opts.WithVolume,
	}

	targetState, targetCfg, err := provisionDatabase(ctx, stateMgr, targetOpts, onProgress)
	if err != nil {
		return nil, nil, err
	}

	report(fmt.Sprintf("Streaming data from '%s' to '%s'...", src.Name, targetState.Name))
	if err := streamCloneData(ctx, dockerMgr, src, targetState); err != nil {
		report("Clone failed; rolling back target database...")
		rollbackProvisionedDatabase(ctx, stateMgr, targetState)
		return nil, nil, err
	}

	return targetState, targetCfg, nil
}

func streamCloneData(ctx context.Context, dockerMgr *container.DockerManager, src, target *state.Database) error {
	var dumpCmd, restoreCmd []string
	if src.Provider == utils.ProviderMySQL {
		dumpCmd = []string{
			"mysqldump",
			"-u", "root",
			fmt.Sprintf("-p%s", src.RootPassword),
			"--single-transaction",
			src.DatabaseName,
		}
		restoreCmd = []string{
			"mysql",
			"-u", "root",
			fmt.Sprintf("-p%s", target.RootPassword),
			target.DatabaseName,
		}
	} else {
		dumpCmd = []string{
			"pg_dump",
			"-U", "postgres",
			"--no-owner",
			"--no-acl",
			src.DatabaseName,
		}
		restoreCmd = []string{
			"psql",
			"-U", "postgres",
			"-d", target.DatabaseName,
			"-v", "ON_ERROR_STOP=1",
		}
	}

	pr, pw := io.Pipe()
	dumpErrCh := make(chan error, 1)

	go func() {
		res, err := dockerMgr.Exec(ctx, src.ContainerID, src.ContainerName, dumpCmd, nil, pw)
		if err != nil {
			_ = pw.CloseWithError(fmt.Errorf("dumping source '%s': %w", src.Name, err))
			dumpErrCh <- err
			return
		}
		if res.ExitCode != 0 {
			dumpErr := fmt.Errorf("dumping source '%s' (exit %d):\n%s", src.Name, res.ExitCode, res.Stderr)
			_ = pw.CloseWithError(dumpErr)
			dumpErrCh <- dumpErr
			return
		}
		_ = pw.Close()
		dumpErrCh <- nil
	}()

	res, restoreErr := dockerMgr.Exec(ctx, target.ContainerID, target.ContainerName, restoreCmd, pr, io.Discard)
	if restoreErr != nil {
		_ = pr.CloseWithError(restoreErr)
		<-dumpErrCh
		return fmt.Errorf("restoring into target '%s': %w", target.Name, restoreErr)
	}
	if res.ExitCode != 0 {
		err := fmt.Errorf("restoring into target '%s' (exit %d):\n%s", target.Name, res.ExitCode, res.Stderr)
		_ = pr.CloseWithError(err)
		<-dumpErrCh
		return err
	}
	_ = pr.Close()

	if dumpErr := <-dumpErrCh; dumpErr != nil {
		return dumpErr
	}
	return nil
}
