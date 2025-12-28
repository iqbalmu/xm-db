package commands

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/container"
	"github.com/iqbalmu/dbm/internal/providers"
	"github.com/iqbalmu/dbm/internal/state"
	"github.com/iqbalmu/dbm/pkg/utils"
	"github.com/spf13/cobra"
)

const (
	defaultMySQLVersion    = "8.0"
	defaultPostgresVersion = "16"
	defaultUsername        = "dev_user"
	containerWaitTimeout   = 30 * time.Second
	containerInitDelay     = 5 * time.Second
)

var (
	createProvider    string
	createVersion     string
	createPort        int
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

		if err := survey.AskOne(&survey.Select{
			Message: "Select database provider:",
			Options: []string{"mysql", "postgres"},
		}, &provider); err != nil {
			color.Red("Error: %v", err)
			return
		}

		defaultVersion := defaultMySQLVersion
		if provider == utils.ProviderPostgres {
			defaultVersion = defaultPostgresVersion
		}

		if err := survey.AskOne(&survey.Input{
			Message: fmt.Sprintf("Version (default: %s):", defaultVersion),
			Default: defaultVersion,
		}, &version); err != nil {
			color.Red("Error: %v", err)
			return
		}

		defaultPort := 3306
		if provider == utils.ProviderPostgres {
			defaultPort = 5432
		}

		var portStr string
		if err := survey.AskOne(&survey.Input{
			Message: fmt.Sprintf("Port (default: %d):", defaultPort),
			Default: fmt.Sprintf("%d", defaultPort),
		}, &portStr); err != nil {
			color.Red("Error: %v", err)
			return
		}
		fmt.Sscanf(portStr, "%d", &port)

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
		// Non-interactive mode
		dbName = args[0]
		provider = createProvider
		version = createVersion
		port = createPort

		if provider == "" {
			color.Red("Error: --provider is required")
			return
		}

		// Generate secure passwords
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
	}

	// Validate
	stateMgr, err := state.NewManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	if stateMgr.Exists(dbName) {
		color.Red("Error: Database '%s' already exists", dbName)
		return
	}

	// Get provider
	var prov providers.Provider
	switch provider {
	case utils.ProviderMySQL:
		prov = providers.NewMySQLProvider()
	case utils.ProviderPostgres:
		prov = providers.NewPostgresProvider()
	default:
		color.Red("Error: Unsupported provider '%s'", provider)
		return
	}

	if port == 0 {
		port = prov.GetDefaultPort()
	}

	// Create database instance
	color.Cyan("\n⏳ Creating %s database '%s'...\n", provider, dbName)

	ctx := context.Background()
	dockerMgr, err := container.NewDockerManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	// Pull image
	imageName := prov.GetImageName(version)
	color.White("📥 Pulling image %s...", imageName)
	if err := dockerMgr.PullImage(ctx, imageName); err != nil {
		color.Red("Error pulling image: %v", err)
		return
	}

	// Create container
	containerName := fmt.Sprintf("dbm_%s", dbName)
	volumeName := fmt.Sprintf("dbm_%s_data", dbName)

	// Determine volume path based on provider
	var volumePath string
	if createWithVolume {
		if provider == utils.ProviderMySQL {
			volumePath = "/var/lib/mysql"
		} else {
			volumePath = "/var/lib/postgresql/data"
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

	color.White("🐳 Creating container...")
	containerID, err := dockerMgr.CreateContainer(ctx, containerConfig)
	if err != nil {
		color.Red("Error creating container: %v", err)
		return
	}

	// Start container
	color.White("▶️  Starting container...")
	if err := dockerMgr.StartContainer(ctx, containerID); err != nil {
		color.Red("Error starting container: %v", err)
		dockerMgr.RemoveContainer(ctx, containerID)
		return
	}

	// Wait for healthy
	color.White("⏳ Waiting for database to be ready...")
	if err := dockerMgr.WaitForHealthy(ctx, containerID, containerWaitTimeout); err != nil {
		color.Red("Error: %v", err)
		dockerMgr.RemoveContainer(ctx, containerID)
		return
	}

	// Give it a bit more time to fully initialize
	time.Sleep(containerInitDelay)

	// Create user if requested
	if createUser && username != "" {
		color.White("👤 Creating user '%s'...", username)

		connStr := prov.GetConnectionString(dbConfig)
		db, err := sql.Open(utils.GetDriverName(provider), connStr)
		if err != nil {
			color.Yellow("Warning: Could not create user: %v", err)
		} else {
			defer db.Close()

			if err := prov.CreateUser(ctx, db, username, userPassword, dbName); err != nil {
				color.Yellow("Warning: Could not create user: %v", err)
			}
		}
	}

	// Save state
	dbState := &state.Database{
		Name:          dbName,
		Provider:      provider,
		Version:       version,
		ContainerName: containerName,
		ContainerID:   containerID,
		Port:          port,
		RootPassword:  rootPassword,
		DatabaseName:  dbName,
		User:          username,
		UserPassword:  userPassword,
		VolumeName:    volumeName,
		CreatedAt:     time.Now(),
		Status:        "running",
	}

	if err := stateMgr.Save(dbState); err != nil {
		color.Red("Error saving state: %v", err)
		return
	}

	// Success message
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
		connStr := utils.BuildConnectionString(provider, username, userPassword, "localhost", port, dbName)
		fmt.Printf("  %s\n", connStr)
	}

	fmt.Println()
}
