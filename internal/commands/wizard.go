package commands

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/state"
)

type wizardAction struct {
	label string
	run   func()
}

// runWizard is the interactive menu shown by `xmdb` with no arguments. Each
// action runs, then the menu redraws until the user exits.
func runWizard() {
	actions := []wizardAction{
		{label: "📊  Show status of all databases", run: func() { runStatus() }},
		{label: "📋  List all databases", run: func() { runList() }},
		{label: "➕  Create a new database", run: func() { runCreate(nil) }},
		{label: "▶️   Start a database", run: func() {
			if name := pickDatabase("Start which database?"); name != "" {
				runStart(name)
			}
		}},
		{label: "⏸️  Stop a database", run: func() {
			if name := pickDatabase("Stop which database?"); name != "" {
				runStop(name)
			}
		}},
		{label: "🗑️  Remove a database", run: func() {
			if name := pickDatabase("Remove which database?"); name != "" {
				runRemove(name)
			}
		}},
		{label: "🔗  Show connection info", run: func() {
			if name := pickDatabase("Show info for which database?"); name != "" {
				runConnect(name)
			}
		}},
		{label: "📄  View database logs", run: func() {
			if name := pickDatabase("Show logs of which database?"); name != "" {
				runLogs(name)
			}
		}},
		{label: "💾  Backup a database", run: func() {
			if name := pickDatabase("Backup which database?"); name != "" {
				runBackup(name)
			}
		}},
		{label: "❓  Show command overview", run: func() { showOverview() }},
		{label: "🚪  Exit", run: nil},
	}

	labels := make([]string, len(actions))
	for i, a := range actions {
		labels[i] = a.label
	}

	for {
		var choice string
		prompt := &survey.Select{
			Message: "What do you want to do?",
			Options: labels,
		}
		if err := survey.AskOne(prompt, &choice); err != nil {
			// Ctrl+C or EOF — leave quietly.
			fmt.Println()
			return
		}

		for _, a := range actions {
			if a.label == choice {
				if a.run == nil {
					return
				}
				a.run()
				break
			}
		}

		// Let the user read the action's output before the menu redraws.
		fmt.Println()
		cont := ""
		_ = survey.AskOne(&survey.Input{Message: "Press Enter to return to the menu…"}, &cont)
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
		color.Yellow("\nNo databases yet — pick 'Create a new database' first.")
		return ""
	}

	options := make([]string, len(dbs))
	for i, d := range dbs {
		options[i] = fmt.Sprintf("%s (%s, %s)", d.Name, d.Provider, groupLabel(d.Group))
	}

	var choice string
	prompt := &survey.Select{Message: question, Options: options}
	if err := survey.AskOne(prompt, &choice); err != nil {
		return ""
	}

	for i, o := range options {
		if o == choice {
			return dbs[i].Name
		}
	}
	return ""
}
