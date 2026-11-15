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

var infoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed information about a database",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runInfo(args[0])
	},
}

func runInfo(name string) {
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

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	ctx := context.Background()

	status := "stopped"
	running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID, db.ContainerName)
	if err == nil && running {
		status = "running"
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

	prov, err := providers.For(db.Provider)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	connStr := prov.GetConnectionString(connectionConfig(db))

	labelColor.Print("Connection String: ")
	valueColor.Println(connStr)

	fmt.Println()
}
