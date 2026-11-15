package commands

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/spf13/cobra"
)

var infoJSON bool

var infoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed information about a database",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInfo(args[0])
	},
}

func init() {
	infoCmd.Flags().BoolVar(&infoJSON, "json", false, "Output as JSON")
}

func runInfo(name string) error {
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

	status := "stopped"
	running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID, db.ContainerName)
	if err == nil && running {
		status = "running"
	}

	prov, err := providers.For(db.Provider)
	if err != nil {
		return err
	}

	connStr := prov.GetConnectionString(connectionConfig(db))

	if infoJSON {
		return printJSON(struct {
			Name             string `json:"name"`
			Provider         string `json:"provider"`
			Group            string `json:"group"`
			Version          string `json:"version"`
			Status           string `json:"status"`
			Port             int    `json:"port"`
			ContainerName    string `json:"container_name"`
			ContainerID      string `json:"container_id"`
			DatabaseName     string `json:"database_name"`
			User             string `json:"user,omitempty"`
			CreatedAt        string `json:"created_at"`
			ConnectionString string `json:"connection_string"`
		}{
			Name:             db.Name,
			Provider:         db.Provider,
			Group:            groupLabel(db.Group),
			Version:          db.Version,
			Status:           status,
			Port:             db.Port,
			ContainerName:    db.ContainerName,
			ContainerID:      db.ContainerID,
			DatabaseName:     db.DatabaseName,
			User:             db.User,
			CreatedAt:        db.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			ConnectionString: connStr,
		})
	}

	titleColor := color.New(color.FgCyan, color.Bold)
	labelColor := color.New(color.FgWhite, color.Bold)
	valueColor := color.New(color.FgWhite)

	fmt.Println()
	titleColor.Printf("Database Information: %s\n", name)
	fmt.Println(color.CyanString("═══════════════════════════════════════"))

	fmt.Println()
	labelColor.Print("Name:           ")
	valueColor.Println(db.Name)

	labelColor.Print("Provider:       ")
	valueColor.Println(db.Provider)

	labelColor.Print("Group:          ")
	valueColor.Println(groupLabel(db.Group))

	labelColor.Print("Version:        ")
	valueColor.Println(db.Version)

	labelColor.Print("Status:         ")
	if status == "running" {
		color.Green(status)
	} else {
		color.Red(status)
	}

	labelColor.Print("Port:           ")
	valueColor.Println(db.Port)

	labelColor.Print("Container Name: ")
	valueColor.Println(db.ContainerName)

	labelColor.Print("Container ID:   ")
	if len(db.ContainerID) > 12 {
		valueColor.Println(db.ContainerID[:12])
	} else {
		valueColor.Println(db.ContainerID)
	}

	labelColor.Print("Database Name:  ")
	valueColor.Println(db.DatabaseName)

	if db.User != "" {
		labelColor.Print("User:           ")
		valueColor.Println(db.User)
	}

	labelColor.Print("Created:        ")
	valueColor.Println(db.CreatedAt.Format("2006-01-02 15:04:05"))

	fmt.Println()
	titleColor.Println("Connection Information:")
	fmt.Println(color.CyanString("───────────────────────────────────────"))

	labelColor.Print("Connection String: ")
	valueColor.Println(connStr)

	fmt.Println()
	return nil
}
