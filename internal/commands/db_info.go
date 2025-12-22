package commands

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/container"
	"github.com/iqbalmu/dbm/internal/state"
	"github.com/spf13/cobra"
)

var dbInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed information about a database",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runDBInfo(args[0])
	},
}

func runDBInfo(name string) {
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

	dockerMgr, err := container.NewDockerManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	ctx := context.Background()

	status := "stopped"
	running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID)
	if err == nil && running {
		status = "running"
	}

	titleColor := color.New(color.FgCyan, color.Bold)
	labelColor := color.New(color.FgWhite, color.Bold)
	valueColor := color.New(color.FgWhite)

	fmt.Println()
	titleColor.Printf("📊 Database Information: %s\n", name)
	fmt.Println(color.CyanString("═══════════════════════════════════════"))

	fmt.Println()
	labelColor.Print("Name:           ")
	valueColor.Println(db.Name)

	labelColor.Print("Provider:       ")
	valueColor.Println(db.Provider)

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
	valueColor.Println(db.ContainerID[:12])

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

	connStr := ""
	if db.Provider == "mysql" {
		connStr = fmt.Sprintf("mysql://%s:%s@localhost:%d/%s",
			db.User, db.UserPassword, db.Port, db.DatabaseName)
	} else {
		connStr = fmt.Sprintf("postgres://%s:%s@localhost:%d/%s",
			db.User, db.UserPassword, db.Port, db.DatabaseName)
	}

	labelColor.Print("Connection String: ")
	valueColor.Println(connStr)

	fmt.Println()
}
