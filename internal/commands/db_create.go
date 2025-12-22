package commands

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/container"
	"github.com/iqbalmu/dbm/internal/providers"
	"github.com/iqbalmu/dbm/internal/state"
	"github.com/spf13/cobra"
)

var (
	createProvider    string
	createVersion     string
	createPort        int
	createInteractive bool
)

var dbCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new database",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runDBCreate(args)
	},
}

func init() {
	dbCreateCmd.Flags().StringVarP(&createProvider, "provider", "p", "", "Database provider (mysql, postgres)")
	dbCreateCmd.Flags().StringVarP(&createVersion, "version", "v", "", "Database version")
	dbCreateCmd.Flags().IntVar(&createPort, "port", 0, "Host port")
	dbCreateCmd.Flags().BoolVarP(&createInteractive, "interactive", "i", false, "Interactive mode")
}

func runDBCreate(args []string) {
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

		defaultVersion := "8.0"
		if provider == "postgres" {
			defaultVersion = "16"
		}

		if err := survey.AskOne(&survey.Input{
			Message: fmt.Sprintf("Version (default: %s):", defaultVersion),
			Default: defaultVersion,
		}, &version); err != nil {
			color.Red("Error: %v", err)
			return
		}

		defaultPort := 3306
		if provider == "postgres" {
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
				Default: "dev_user",
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

		rootPassword = generatePassword()
		username = "dev_user"
		userPassword = generatePassword()
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
	case "mysql":
		prov = providers.NewMySQLProvider()
	case "postgres":
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
	if err := dockerMgr.WaitForHealthy(ctx, containerID, 30*time.Second); err != nil {
		color.Red("Error: %v", err)
		dockerMgr.RemoveContainer(ctx, containerID)
		return
	}

	// Give it a bit more time to fully initialize
	time.Sleep(5 * time.Second)

	// Create user if requested
	if createUser && username != "" {
		color.White("👤 Creating user '%s'...", username)

		connStr := prov.GetConnectionString(dbConfig)
		db, err := sql.Open(getDriverName(provider), connStr)
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

		if provider == "mysql" {
			color.White("Connection String:")
			fmt.Printf("  mysql://%s:%s@localhost:%d/%s\n", username, userPassword, port, dbName)
		} else {
			color.White("Connection String:")
			fmt.Printf("  postgres://%s:%s@localhost:%d/%s\n", username, userPassword, port, dbName)
		}
	}

	fmt.Println()
}

func generatePassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func getDriverName(provider string) string {
	if provider == "postgres" {
		return "postgres"
	}
	return "mysql"
}
