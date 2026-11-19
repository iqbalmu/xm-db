package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/iqbalmu/xmdb/internal/container"
	"github.com/iqbalmu/xmdb/internal/state"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	statusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show status of all managed databases",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
	statusGroup string
	statusJSON  bool
)

func init() {
	statusCmd.Flags().StringVarP(&statusGroup, "group", "g", "", "Filter by group ('global' for ungrouped databases)")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
}

func runStatus() error {
	stateMgr, err := state.NewManager()
	if err != nil {
		return err
	}

	databases, err := stateMgr.List()
	if err != nil {
		return fmt.Errorf("loading databases: %w", err)
	}

	if total := len(databases); total > 0 {
		databases = filterByGroup(databases, statusGroup)
		if len(databases) == 0 {
			if statusJSON {
				return printJSON([]dbListItem{})
			}
			color.Yellow("\nNo databases in group '%s'.", statusGroup)
			return nil
		}
	}

	if len(databases) == 0 {
		if statusJSON {
			return printJSON([]dbListItem{})
		}
		color.Yellow("No databases found. Create one with: xmdb create <name>")
		return nil
	}

	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		return fmt.Errorf("connecting to Docker: %w", err)
	}

	ctx := context.Background()

	items := make([]dbListItem, 0, len(databases))
	var data [][]any
	for _, db := range databases {
		status := "stopped"
		if db.ContainerID != "" {
			running, err := dockerMgr.IsContainerRunning(ctx, db.ContainerID, db.ContainerName)
			if err == nil && running {
				status = "running"
			}
		}

		items = append(items, dbListItem{
			Name:      db.Name,
			Provider:  db.Provider,
			Version:   db.Version,
			Group:     groupLabel(db.Group),
			Status:    status,
			Port:      db.Port,
			CreatedAt: db.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})

		data = append(data, []any{
			db.Name,
			db.Provider,
			groupLabel(db.Group),
			status,
			db.Port,
			db.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	if statusJSON {
		return printJSON(items)
	}

	fmt.Println()

	table := tablewriter.NewTable(os.Stdout)
	table.Header("Name", "Provider", "Group", "Status", "Port", "Created")
	table.Bulk(data)
	table.Render()

	fmt.Println()
	return nil
}
