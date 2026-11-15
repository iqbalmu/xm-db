package commands

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/state"
)

type wizardAction struct {
	label string
	run   func()
}

// runWizard is the interactive menu shown by `xmdb` with no arguments.
// One pass: pick an action (and optionally a database), run it, and the
// program exits. Esc backs out of any menu (at the top menu it exits),
// Ctrl+C exits too.
func runWizard() {
	groups := []wizardGroup{
		{
			label: "Databases",
			actions: []wizardAction{
				{label: "Create a new database", run: func() { runCreate(nil) }},
				{label: "List all databases", run: func() { runList() }},
				{label: "Show status of all databases", run: func() { runStatus() }},
				{label: "Start a database", run: pickAndRun("Start which database?", runStart)},
				{label: "Stop a database", run: pickAndRun("Stop which database?", runStop)},
				{label: "Remove a database", run: pickAndRun("Remove which database?", runRemove)},
			},
		},
		{
			label: "Connect & maintenance",
			actions: []wizardAction{
				{label: "Show connection info", run: pickAndRun("Show info for which database?", runConnect)},
				{label: "View database logs", run: pickAndRun("Show logs of which database?", runLogs)},
				{label: "Backup a database", run: pickAndRun("Backup which database?", runBackup)},
			},
		},
	}

	topOptions := make([]string, 0, len(groups)+2)
	for _, g := range groups {
		topOptions = append(topOptions, g.label)
	}
	topOptions = append(topOptions, "Command overview", "Exit")

	// One pass: pick an action, run it, and the program exits.
	choice, ok := promptSelect("What do you want to do?", topOptions)
	if !ok || choice == "Exit" {
		fmt.Println()
		return
	}

	if choice == "Command overview" {
		showOverview()
		return
	}

	for _, g := range groups {
		if g.label == choice {
			runGroupMenu(g)
			return
		}
	}
}

type wizardGroup struct {
	label   string
	actions []wizardAction
}

// runGroupMenu shows one group's actions; the chosen action runs once and
// the wizard ends. Esc or Back just skips the action.
func runGroupMenu(group wizardGroup) {
	options := make([]string, 0, len(group.actions)+1)
	for _, a := range group.actions {
		options = append(options, a.label)
	}
	options = append(options, "Back")

	choice, ok := promptSelect(group.label, options)
	if !ok || choice == "Back" {
		return
	}

	for _, a := range group.actions {
		if a.label == choice {
			a.run()
			return
		}
	}
}

func pickAndRun(question string, fn func(string)) func() {
	return func() {
		if name := pickDatabase(question); name != "" {
			fn(name)
		}
	}
}

// pickDatabase shows a selector over the managed databases and returns the
// chosen name, or "" when there is nothing to pick or the user cancels.
func pickDatabase(question string) string {
	stateMgr, err := state.NewManager()
	if err != nil {
		color.Red("Error: %v", err)
		return ""
	}

	dbs, err := stateMgr.List()
	if err != nil {
		color.Red("Error loading databases: %v", err)
		return ""
	}
	if len(dbs) == 0 {
		color.Yellow("\nNo databases yet. Pick 'Create a new database' first.")
		return ""
	}

	options := make([]string, len(dbs))
	for i, d := range dbs {
		options[i] = fmt.Sprintf("%s (%s, %s)", d.Name, d.Provider, groupLabel(d.Group))
	}

	choice, ok := promptSelect(question, options)
	if !ok {
		return ""
	}

	for i, o := range options {
		if o == choice {
			return dbs[i].Name
		}
	}
	return ""
}
