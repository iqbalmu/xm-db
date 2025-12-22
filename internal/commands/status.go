package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/iqbalmu/dbm/internal/container"
	"github.com/iqbalmu/dbm/internal/state"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all managed databases",
	Run: func(cmd *cobra.Command, args []string) {
		runStatus()
	},
}

func runStatus() {
	stateMgr, err := state.NewManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	databases, err := stateMgr.List()
	if err != nil {
		color.Red("Error loading databases: %v", err)
		return
	}

	if len(databases) == 0 {
		color.Yellow("No databases found. Create one with: dbm db create <name>")
		return
	}

	dockerMgr, err := container.NewDockerManager()
	if err != nil {
		color.Red("Error connecting to Docker: %v", err)
		return
	}

	ctx := context.Background()

	// Prepare data
	var data [][]any
	for _, db := range databases {
		status := "stopped"
		if db.ContainerID != "" {
			running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID)
			if err == nil && running {
				status = "running"
			}
		}

		createdAt := db.CreatedAt.Format("2006-01-02 15:04")

		data = append(data, []any{
			db.Name,
			db.Provider,
			status,
			db.Port,
			createdAt,
		})
	}

	fmt.Println()

	// New API v1+
	table := tablewriter.NewTable(os.Stdout)
	table.Header("Name", "Provider", "Status", "Port", "Created")
	table.Bulk(data)
	table.Render()

	fmt.Println()
}
