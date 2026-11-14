package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	statusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show status of all managed databases",
		Run: func(cmd *cobra.Command, args []string) {
			runStatus()
		},
	}
	statusGroup string
)

func init() {
	statusCmd.Flags().StringVarP(&statusGroup, "group", "g", "", "Filter by group ('global' for ungrouped databases)")
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

	if total := len(databases); total > 0 {
		databases = filterByGroup(databases, statusGroup)
		if len(databases) == 0 {
			color.Yellow("\nNo databases in group '%s'.", statusGroup)
			return
		}
	}

	if len(databases) == 0 {
		color.Yellow("No databases found. Create one with: xmdb create <name>")
		return
	}

	dockerMgr, err := container.NewDockerManager(runtimeMode)
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
			groupLabel(db.Group),
			status,
			db.Port,
			createdAt,
		})
	}

	fmt.Println()

	table := tablewriter.NewTable(os.Stdout)
	table.Header("Name", "Provider", "Group", "Status", "Port", "Created")
	table.Bulk(data)
	table.Render()

	fmt.Println()
}
