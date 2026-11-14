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

// runWizard is the interactive menu shown by `xmdb` with no arguments.
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

	for {
		choice, ok := promptSelect("What do you want to do?", topOptions)
		if !ok || choice == "Exit" {
			fmt.Println()
			return
		}

		if choice == "Command overview" {
			showOverview()
			waitForEnter()
			continue
		}

		for _, g := range groups {
			if g.label == choice {
				runGroupMenu(g)
				break
			}
		}
	}
}

type wizardGroup struct {
	label   string
	actions []wizardAction
}

func runGroupMenu(group wizardGroup) {
	options := make([]string, 0, len(group.actions)+1)
	for _, a := range group.actions {
		options = append(options, a.label)
	}
	options = append(options, "Back")

	for {
		choice, ok := promptSelect(group.label, options)
		if !ok || choice == "Back" {
			return
		}

		for _, a := range group.actions {
			if a.label == choice {
				a.run()
				waitForEnter()
				break
			}
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

func promptSelect(message string, options []string) (string, bool) {
	var choice string
	prompt := &survey.Select{
		Message: message,
		Options: options,
	}
	if err := survey.AskOne(prompt, &choice); err != nil {
		return "", false
	}
	return choice, true
}

func waitForEnter() {
	fmt.Println()
	cont := ""
	_ = survey.AskOne(&survey.Input{Message: "Press Enter to return to the menu..."}, &cont)
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
