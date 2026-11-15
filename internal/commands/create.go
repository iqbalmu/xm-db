package commands

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/iqbalmu/xm-db/pkg/utils"
	"github.com/spf13/cobra"
)

const (
	defaultUsername       = "dev_user"
	containerStartTimeout = 60 * time.Second
	portProbeTimeout      = 500 * time.Millisecond
	connectAttemptTimeout = 5 * time.Second
)

var (
	createProvider    string
	createVersion     string
	createPort        int
	createGroup       string
	createInteractive bool
	createWithVolume  bool
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new database",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCreate(args)
	},
}

func init() {
	createCmd.Flags().StringVarP(&createProvider, "provider", "p", "", "Database provider (mysql, postgres)")
	createCmd.Flags().StringVarP(&createVersion, "version", "v", "", "Database version")
	createCmd.Flags().IntVar(&createPort, "port", 0, "Host port")
	createCmd.Flags().StringVarP(&createGroup, "group", "g", "", "Group the database belongs to (empty = global)")
	createCmd.Flags().BoolVarP(&createInteractive, "interactive", "i", false, "Interactive mode")
	createCmd.Flags().BoolVar(&createWithVolume, "volume", true, "Create with persistent volume (default: true)")
}

func runCreate(args []string) {
	var dbName string
	var provider string
	var version string
	var port int
	var rootPassword string
	var createUser bool
	var username string
	var userPassword string

	// Interactive mode
	if createInteractive || len(args) == 0 {
		if err := survey.AskOne(&survey.Input{
			Message: "Database name:",
		}, &dbName, survey.WithValidator(survey.Required)); err != nil {
			color.Red("Error: %v", err)
			return
		}

		if createProvider == "" {
			if err := survey.AskOne(&survey.Select{
				Message: "Select database provider:",
				Options: []string{"mysql", "postgres"},
			}, &provider); err != nil {
				color.Red("Error: %v", err)
				return
			}
		} else {
			provider = createProvider
		}

		defaultVersion := providers.DefaultMySQLVersion
		if provider == utils.ProviderPostgres {
			defaultVersion = providers.DefaultPostgresVersion
		}

		if err := survey.AskOne(&survey.Input{
			Message: fmt.Sprintf("Version (default: %s):", defaultVersion),
			Default: defaultVersion,
		}, &version); err != nil {
			color.Red("Error: %v", err)
			return
		}

		defaultPort := providers.DefaultMySQLPort
		if provider == utils.ProviderPostgres {
			defaultPort = providers.DefaultPostgresPort
		}
		if createPort != 0 {
			defaultPort = createPort
		}

		var portStr string
		if err := survey.AskOne(&survey.Input{
			Message: fmt.Sprintf("Port (default: %d):", defaultPort),
			Default: strconv.Itoa(defaultPort),
		}, &portStr, survey.WithValidator(survey.ComposeValidators(survey.Required, survey.Validator(func(ans interface{}) error {
			if _, err := strconv.Atoi(ans.(string)); err != nil {
				return fmt.Errorf("port must be a number")
			}
			return nil
		})))); err != nil {
			color.Red("Error: %v", err)
			return
		}
		parsedPort, err := strconv.Atoi(portStr)
		if err != nil {
			color.Red("Error: invalid port '%s'", portStr)
			return
		}
		port = parsedPort

		if err := survey.AskOne(&survey.Input{
			Message: "Group (Enter for global):",
		}, &createGroup); err != nil {
			color.Red("Error: %v", err)
			return
		}

		if err := survey.AskOne(&survey.Password{
			Message: "Root password:",
		}, &rootPassword, survey.WithValidator(survey.Required)); err != nil {
			color.Red("Error: %v", err)
			return
		}

		if err := survey.AskOne(&survey.Confirm{
			Message: "Create a user?",
			Default: true,
		}, &createUser); err != nil {
			color.Red("Error: %v", err)
			return
		}

		if createUser {
			if err := survey.AskOne(&survey.Input{
				Message: "Username:",
				Default: defaultUsername,
			}, &username); err != nil {
				color.Red("Error: %v", err)
				return
			}

			if err := survey.AskOne(&survey.Password{
				Message: "User password:",
			}, &userPassword, survey.WithValidator(survey.Required)); err != nil {
				color.Red("Error: %v", err)
				return
			}
		}
	} else {
		dbName = args[0]
		provider = createProvider
		version = createVersion
		port = createPort

		if provider == "" {
			color.Red("Error: --provider is required")
			return
		}

		var err error
		rootPassword, err = utils.GeneratePassword(utils.DefaultPasswordLength)
		if err != nil {
			color.Red("Error generating password: %v", err)
			return
		}

		userPassword, err = utils.GeneratePassword(utils.DefaultPasswordLength)
		if err != nil {
			color.Red("Error generating password: %v", err)
			return
		}

		username = defaultUsername
		createUser = true

		if version == "" {
			version = providers.DefaultMySQLVersion
			if provider == utils.ProviderPostgres {
				version = providers.DefaultPostgresVersion
			}
		}
	}

	stateMgr, err := state.NewManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	if stateMgr.Exists(dbName) {
		color.Red("Error: Database '%s' already exists", dbName)
		return
	}

	prov, err := providers.For(provider)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	if port == 0 {
		port = prov.GetDefaultPort()
	}

	// Refuse ports taken by another managed database or by any local process.
	if owner := managedPortOwner(stateMgr, port); owner != "" {
		color.Red("Error: Port %d is already used by database '%s'", port, owner)
		return
	}
	if portInUse(port) {
		color.Red("Error: Port %d is already in use by another process", port)
		return
	}

	color.Cyan("\n⏳ Creating %s database '%s'...\n", provider, dbName)

	ctx := context.Background()
	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	imageName := prov.GetImageName(version)
	color.White("📥 Pulling image %s...", imageName)
	if err := dockerMgr.PullImage(ctx, imageName); err != nil {
		color.Red("Error pulling image: %v", err)
		return
	}

	containerName := fmt.Sprintf("xmdb_%s", dbName)
	volumeName := ""
	var volumePath string
	if createWithVolume {
		volumeName = fmt.Sprintf("xmdb_%s_data", dbName)

		if provider == utils.ProviderMySQL {
			volumePath = "/var/lib/mysql"
		} else {
			volumePath = "/var/lib/postgresql/data"
		}

		if exists, err := dockerMgr.VolumeExists(ctx, volumeName); err == nil && exists {
			color.Yellow("⚠️  Reusing existing volume '%s' (contains data from a previous database)", volumeName)
		}
	}

	dbConfig := &providers.DBConfig{
		Host:         "localhost",
		Port:         port,
		RootPassword: rootPassword,
		DatabaseName: dbName,
		User:         username,
		UserPassword: userPassword,
	}

	containerConfig := &container.ContainerConfig{
		Name:          containerName,
		Image:         imageName,
		Port:          port,
		ContainerPort: prov.GetContainerPort(),
		Env:           prov.GetEnvVars(dbConfig),
		VolumeName:    volumeName,
		VolumePath:    volumePath,
	}

	// Admin connection for readiness checks and user provisioning: the dev
	// user does not exist until CreateUser runs.
	adminConfig := &providers.DBConfig{
		Host:         "localhost",
		Port:         port,
		RootPassword: rootPassword,
		DatabaseName: dbName,
	}

	color.White("🐳 Creating container...")
	containerID, err := dockerMgr.CreateContainer(ctx, containerConfig)
	if err != nil {
		color.Red("Error creating container: %v", err)
		return
	}

	color.White("▶️  Starting container...")
	if err := dockerMgr.StartContainer(ctx, containerID, containerName); err != nil {
		color.Red("Error starting container: %v", err)
		cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
		return
	}

	// Wait until the server answers a real protocol handshake. Rootless
	// container runtimes accept host-side TCP connections before the database
	// listens, so a bare port dial is not proof of readiness.
	color.White("⏳ Waiting for database to accept connections...")
	if err := waitForDatabase(ctx, prov, adminConfig, containerStartTimeout); err != nil {
		color.Red("Error: %v", err)
		cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
		return
	}

	// Create user if requested. Fail hard instead of saving credentials that
	// do not exist.
	if createUser && username != "" {
		color.White("👤 Creating user '%s'...", username)

		db, err := prov.Connect(adminConfig)
		if err != nil {
			color.Red("Error connecting to database: %v", err)
			cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
			return
		}

		if err := prov.CreateUser(ctx, db, username, userPassword, dbName); err != nil {
			db.Close()
			color.Red("Error creating user: %v", err)
			cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
			return
		}
		db.Close()
	}

	dbState := &state.Database{
		Name:          dbName,
		Provider:      provider,
		Version:       version,
		Group:         strings.TrimSpace(createGroup),
		ContainerName: containerName,
		ContainerID:   containerID,
		Port:          port,
		RootPassword:  rootPassword,
		DatabaseName:  dbName,
		User:          username,
		UserPassword:  userPassword,
		VolumeName:    volumeName,
		CreatedAt:     time.Now(),
	}

	if err := stateMgr.Save(dbState); err != nil {
		color.Red("Error saving state: %v", err)
		return
	}

	color.Green("\n✓ Database '%s' created successfully!\n", dbName)

	fmt.Println("Connection Information:")
	color.Cyan("  Provider: %s", provider)
	color.Cyan("  Port: %d", port)
	color.Cyan("  Database: %s", dbName)

	if createUser && username != "" {
		color.Cyan("  User: %s", username)
		color.Cyan("  Password: %s", userPassword)
		fmt.Println()

		color.White("Connection String:")
		fmt.Printf("  %s\n", prov.GetConnectionString(dbConfig))
	}

	fmt.Println()
}

// waitForDatabase polls until the server completes a protocol-level
// handshake. Each attempt is bounded by its own timeout, so a hung dial
// cannot stall past the overall deadline.
func waitForDatabase(ctx context.Context, prov providers.Provider, cfg *providers.DBConfig, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
		attemptCtx, cancel := context.WithTimeout(ctx, connectAttemptTimeout)
		db, err := prov.Connect(cfg)
		if err != nil {
			lastErr = err
		} else {
			lastErr = db.PingContext(attemptCtx)
			db.Close()
		}
		cancel()
		if lastErr == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %v waiting for database (last error: %w)", timeout, lastErr)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// cleanup removes a failed creation's container and data volume. Failures
// are reported, not swallowed: a stuck container blocks the next create
// with the same name.
func cleanup(dockerMgr *container.DockerManager, ctx context.Context, containerID, containerName, volumeName string) {
	if err := dockerMgr.RemoveContainer(ctx, containerID, containerName); err != nil {
		color.Yellow("  Warning: could not clean up container '%s': %v", containerName, err)
	}
	if volumeName != "" {
		if err := dockerMgr.RemoveVolume(ctx, volumeName); err != nil {
			color.Yellow("  Warning: could not clean up volume '%s': %v", volumeName, err)
		}
	}
}

// managedPortOwner returns the name of the managed database bound to port.
func managedPortOwner(m *state.Manager, port int) string {
	dbs, err := m.List()
	if err != nil {
		return ""
	}
	for _, d := range dbs {
		if d.Port == port {
			return d.Name
		}
	}
	return ""
}

// portInUse reports whether a local process is already listening on the port.
func portInUse(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), portProbeTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
