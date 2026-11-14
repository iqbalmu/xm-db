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
	listCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all managed databases",
		Run: func(cmd *cobra.Command, args []string) {
			runList()
		},
	}
	listGroup string
)

func init() {
	listCmd.Flags().StringVarP(&listGroup, "group", "g", "", "Filter by group ('global' for ungrouped databases)")
}

func runList() {
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
		databases = filterByGroup(databases, listGroup)
		if len(databases) == 0 {
			color.Yellow("\nNo databases in group '%s'.", listGroup)
			fmt.Println()
			return
		}
	}

	if len(databases) == 0 {
		color.Yellow("\nNo databases found.")
		fmt.Println("Create one with: xmdb create <name> --provider <mysql|postgres>")
		fmt.Println()
		return
	}

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		color.Red("Error connecting to Docker: %v", err)
		return
	}

	ctx := context.Background()

	color.Cyan("\n📊 Managed Databases:\n")

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
			db.Version,
			groupLabel(db.Group),
			status,
			db.Port,
			createdAt,
		})
	}

	table := tablewriter.NewTable(os.Stdout)
	table.Header("Name", "Provider", "Version", "Group", "Status", "Port", "Created")
	table.Bulk(data)
	table.Render()

	fmt.Println()
}
