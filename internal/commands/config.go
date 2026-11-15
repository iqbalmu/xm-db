package commands

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration file with defaults",
	Run: func(cmd *cobra.Command, args []string) {
		runConfigInit()
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Run: func(cmd *cobra.Command, args []string) {
		runConfigShow()
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	Run: func(cmd *cobra.Command, args []string) {
		runConfigPath()
	},
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathCmd)
}

func runConfigInit() {
	if config.Exists() {
		color.Yellow("Configuration file already exists")
		path, _ := config.GetConfigPath()
		fmt.Printf("Path: %s\n", path)
		fmt.Println("\nUse 'xmdb config show' to view current configuration")
		return
	}

	cfg := config.DefaultConfig()
	if err := cfg.Save(); err != nil {
		color.Red("Error: %v", err)
		return
	}

	path, _ := config.GetConfigPath()
	color.Green("\nConfiguration file created successfully!")
	color.Cyan("  Path: %s", path)
	fmt.Println("\nDefault settings:")
	displayConfig(cfg)
}

func runConfigShow() {
	cfg, err := config.Load()
	if err != nil {
		color.Red("Error loading configuration: %v", err)
		return
	}

	color.Cyan("\nCurrent Configuration:\n")
	fmt.Println(color.CyanString("════════════════════════════════════════"))
	displayConfig(cfg)
}

func runConfigPath() {
	path, err := config.GetConfigPath()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	fmt.Println(path)

	if !config.Exists() {
		color.Yellow("\nConfiguration file does not exist yet")
		fmt.Println("Run 'xmdb config init' to create it")
	}
}

func displayConfig(cfg *config.Config) {
	labelColor := color.New(color.FgWhite, color.Bold)

	fmt.Println()
	labelColor.Print("Default Provider:  ")
	fmt.Println(cfg.DefaultProvider)

	labelColor.Print("Volume Enabled:    ")
	fmt.Println(cfg.VolumeEnabled)

	labelColor.Print("Backup Directory:  ")
	fmt.Println(cfg.BackupDir)

	fmt.Println()
	labelColor.Println("Default Versions:")
	for provider, version := range cfg.DefaultVersion {
		fmt.Printf("  %s: %s\n", provider, version)
	}

	fmt.Println()
	labelColor.Println("Default Ports:")
	for provider, port := range cfg.DefaultPort {
		fmt.Printf("  %s: %d\n", provider, port)
	}

	fmt.Println()
}
