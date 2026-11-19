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
	listCmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all managed databases",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList()
		},
	}
	listGroup string
	listJSON  bool
)

func init() {
	listCmd.Flags().StringVarP(&listGroup, "group", "g", "", "Filter by group ('global' for ungrouped databases)")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
}

type dbListItem struct {
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Version   string `json:"version"`
	Group     string `json:"group"`
	Status    string `json:"status"`
	Port      int    `json:"port"`
	CreatedAt string `json:"created_at"`
}

func runList() error {
	stateMgr, err := state.NewManager()
	if err != nil {
		return err
	}

	databases, err := stateMgr.List()
	if err != nil {
		return fmt.Errorf("loading databases: %w", err)
	}

	if total := len(databases); total > 0 {
		databases = filterByGroup(databases, listGroup)
		if len(databases) == 0 {
			if listJSON {
				return printJSON([]dbListItem{})
			}
			color.Yellow("\nNo databases in group '%s'.", listGroup)
			fmt.Println()
			return nil
		}
	}

	if len(databases) == 0 {
		if listJSON {
			return printJSON([]dbListItem{})
		}
		color.Yellow("\nNo databases found.")
		fmt.Println("Create one with: xmdb create <name> --provider <mysql|postgres>")
		fmt.Println()
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
			db.Version,
			groupLabel(db.Group),
			status,
			db.Port,
			db.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	if listJSON {
		return printJSON(items)
	}

	color.Cyan("\nManaged Databases:\n")

	table := tablewriter.NewTable(os.Stdout)
	table.Header("Name", "Provider", "Version", "Group", "Status", "Port", "Created")
	table.Bulk(data)
	table.Render()

	fmt.Println()
	return nil
}
