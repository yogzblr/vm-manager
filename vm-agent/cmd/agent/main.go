// Package main provides the entry point for the vm-agent.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/yourorg/vm-agent/internal/version"
	"github.com/yourorg/vm-agent/pkg/agent"
	"github.com/yourorg/vm-agent/pkg/config"
	"github.com/yourorg/vm-agent/pkg/lifecycle"
)

var (
	cfgFile string
	dataDir string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "vm-agent",
	Short: "Multi-Tenant VM Management Agent",
	Long: `vm-agent is a lightweight agent for VM management that provides:
- Remote workflow execution via Piko tunneling
- Health monitoring and reporting
- Self-upgrade capabilities
- Multi-tenant isolation`,
	Version: version.Version,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "/etc/vm-agent/config.yaml", "config file path")
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "/var/lib/vm-agent", "data directory path")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(repairCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(versionCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the agent",
	Long:  "Start the vm-agent and begin accepting connections",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		loader := config.NewLoader()
		loader.SetConfigPath(cfgFile)

		cfg, err := loader.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		// Validate configuration
		validator := config.NewValidator()
		if err := validator.Validate(cfg); err != nil {
			return fmt.Errorf("configuration validation failed: %w", err)
		}

		// Create and run manager
		mgr, err := agent.NewManager(cfg)
		if err != nil {
			return fmt.Errorf("failed to create manager: %w", err)
		}

		return mgr.Run()
	},
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the agent",
	Long:  "Install the vm-agent as a system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		tenantID, _ := cmd.Flags().GetString("tenant-id")
		installKey, _ := cmd.Flags().GetString("key")
		pikoURL, _ := cmd.Flags().GetString("piko-url")
		controlPlaneURL, _ := cmd.Flags().GetString("control-plane-url")
		agentID, _ := cmd.Flags().GetString("agent-id")

		if tenantID == "" {
			return fmt.Errorf("--tenant-id is required")
		}
		if installKey == "" {
			return fmt.Errorf("--key is required")
		}

		// Create logger
		logger, _ := initBasicLogger()

		installer := lifecycle.NewInstaller(&lifecycle.InstallerConfig{
			DataDir:         dataDir,
			ConfigPath:      cfgFile,
			ControlPlaneURL: controlPlaneURL,
		}, logger)

		opts := &lifecycle.InstallOptions{
			TenantID:        tenantID,
			InstallationKey: installKey,
			PikoServerURL:   pikoURL,
			ControlPlaneURL: controlPlaneURL,
			AgentID:         agentID,
		}

		if err := installer.Install(context.Background(), opts); err != nil {
			return fmt.Errorf("installation failed: %w", err)
		}

		fmt.Println("Agent installed successfully")
		return nil
	},
}

func initInstallCmd() {
	installCmd.Flags().String("tenant-id", "", "Tenant ID")
	installCmd.Flags().String("key", "", "Installation key")
	installCmd.Flags().String("piko-url", "", "Piko server URL")
	installCmd.Flags().String("control-plane-url", "", "Control plane URL")
	installCmd.Flags().String("agent-id", "", "Agent ID (defaults to hostname)")
}

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure the agent",
	Long:  "Update agent configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger, _ := initBasicLogger()
		configurator := lifecycle.NewConfigurator(cfgFile, logger)

		if err := configurator.ConfigureFromEnv(); err != nil {
			return fmt.Errorf("configuration failed: %w", err)
		}

		fmt.Println("Configuration updated successfully")
		return nil
	},
}

var repairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Repair the agent",
	Long:  "Diagnose and repair agent issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		diagnoseOnly, _ := cmd.Flags().GetBool("diagnose")
		logger, _ := initBasicLogger()

		repairer := lifecycle.NewRepairer(dataDir, cfgFile, logger)

		var result *lifecycle.RepairResult
		var err error

		if diagnoseOnly {
			result, err = repairer.Diagnose(context.Background())
		} else {
			result, err = repairer.Repair(context.Background())
		}

		if err != nil {
			return fmt.Errorf("repair failed: %w", err)
		}

		// Print results
		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(output))

		if !result.Success {
			return fmt.Errorf("repair completed with failures")
		}

		return nil
	},
}

func initRepairCmd() {
	repairCmd.Flags().Bool("diagnose", false, "Only diagnose issues, don't repair")
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the agent",
	Long:  "Upgrade the agent to a new version",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetVersion, _ := cmd.Flags().GetString("version")
		downloadURL, _ := cmd.Flags().GetString("url")
		checksum, _ := cmd.Flags().GetString("checksum")

		if targetVersion == "" || downloadURL == "" || checksum == "" {
			return fmt.Errorf("--version, --url, and --checksum are required")
		}

		logger, _ := initBasicLogger()
		upgrader := lifecycle.NewUpgrader(dataDir, logger)

		if err := upgrader.StartUpgrade(targetVersion, downloadURL, checksum); err != nil {
			return fmt.Errorf("upgrade failed: %w", err)
		}

		fmt.Println("Upgrade started")
		return nil
	},
}

func initUpgradeCmd() {
	upgradeCmd.Flags().String("version", "", "Target version")
	upgradeCmd.Flags().String("url", "", "Download URL")
	upgradeCmd.Flags().String("checksum", "", "SHA256 checksum")
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the agent",
	Long:  "Uninstall the vm-agent from the system",
	RunE: func(cmd *cobra.Command, args []string) error {
		keepData, _ := cmd.Flags().GetBool("keep-data")
		keepConfig, _ := cmd.Flags().GetBool("keep-config")
		keepLogs, _ := cmd.Flags().GetBool("keep-logs")
		purge, _ := cmd.Flags().GetBool("purge")

		logger, _ := initBasicLogger()
		uninstaller := lifecycle.NewUninstaller(dataDir, cfgFile, logger)

		if purge {
			if err := uninstaller.Purge(context.Background()); err != nil {
				return fmt.Errorf("purge failed: %w", err)
			}
			fmt.Println("Agent purged successfully")
			return nil
		}

		opts := &lifecycle.UninstallOptions{
			KeepData:   keepData,
			KeepConfig: keepConfig,
			KeepLogs:   keepLogs,
		}

		result, err := uninstaller.Uninstall(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("uninstall failed: %w", err)
		}

		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(output))

		if !result.Success {
			return fmt.Errorf("uninstall completed with errors")
		}

		return nil
	},
}

func initUninstallCmd() {
	uninstallCmd.Flags().Bool("keep-data", false, "Keep data directory")
	uninstallCmd.Flags().Bool("keep-config", false, "Keep configuration file")
	uninstallCmd.Flags().Bool("keep-logs", false, "Keep log files")
	uninstallCmd.Flags().Bool("purge", false, "Remove all agent files including binary")
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show agent status",
	Long:  "Display current agent status and health information",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger, _ := initBasicLogger()
		installer := lifecycle.NewInstaller(&lifecycle.InstallerConfig{
			DataDir:    dataDir,
			ConfigPath: cfgFile,
		}, logger)

		info, err := installer.GetInstallInfo()
		if err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}

		output, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(output))
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  "Display version and build information",
	Run: func(cmd *cobra.Command, args []string) {
		info := version.GetInfo()
		fmt.Println(info.String())
	},
}

func initBasicLogger() (*zap.Logger, error) {
	return zap.NewProduction()
}

func init() {
	initInstallCmd()
	initRepairCmd()
	initUpgradeCmd()
	initUninstallCmd()
}
