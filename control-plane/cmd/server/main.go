// Package main provides the control plane server entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/yourorg/control-plane/internal/version"
	"github.com/yourorg/control-plane/pkg/agent"
	"github.com/yourorg/control-plane/pkg/api"
	"github.com/yourorg/control-plane/pkg/audit"
	"github.com/yourorg/control-plane/pkg/auth"
	"github.com/yourorg/control-plane/pkg/campaign"
	"github.com/yourorg/control-plane/pkg/db"
	"github.com/yourorg/control-plane/pkg/mcp"
	"github.com/yourorg/control-plane/pkg/tenant"
	"github.com/yourorg/control-plane/pkg/workflow"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "control-plane",
		Short: "VM Manager Control Plane",
		Long:  `Control plane for multi-tenant VM management system.`,
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/control-plane/")
	}

	viper.SetEnvPrefix("CP")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
		}
	}
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the control plane server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer()
	},
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server (stdio)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMCP()
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrations()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version: %s\n", version.Version)
		fmt.Printf("Commit: %s\n", version.Commit)
		fmt.Printf("Build Date: %s\n", version.BuildDate)
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServer() error {
	// Initialize logger
	logger, err := createLogger()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("starting control plane server",
		zap.String("version", version.Version))

	// Initialize database
	dbConfig := &db.Config{
		Host:            viper.GetString("database.host"),
		Port:            viper.GetInt("database.port"),
		User:            viper.GetString("database.user"),
		Password:        viper.GetString("database.password"),
		Database:        viper.GetString("database.name"),
		MaxOpenConns:    viper.GetInt("database.max_open_conns"),
		MaxIdleConns:    viper.GetInt("database.max_idle_conns"),
		ConnMaxLifetime: viper.GetDuration("database.conn_max_lifetime"),
	}

	if dbConfig.Host == "" {
		dbConfig.Host = "localhost"
	}
	if dbConfig.Port == 0 {
		dbConfig.Port = 3306
	}
	if dbConfig.User == "" {
		dbConfig.User = "root"
	}
	if dbConfig.Database == "" {
		dbConfig.Database = "vmmanager"
	}

	database, err := db.NewConnection(dbConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Run migrations
	if err := db.RunMigrations(database, logger); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Initialize JWT auth
	jwtSecret := viper.GetString("auth.jwt_secret")
	if jwtSecret == "" {
		jwtSecret = "default-secret-change-in-production"
		logger.Warn("using default JWT secret, change in production!")
	}

	jwtAuth := auth.NewJWTAuth(&auth.JWTConfig{
		Secret:          jwtSecret,
		Issuer:          viper.GetString("auth.issuer"),
		TokenExpiry:     viper.GetDuration("auth.token_expiry"),
		RefreshExpiry:   viper.GetDuration("auth.refresh_expiry"),
	})

	// Initialize managers
	tenantManager := tenant.NewManager(database, logger)
	agentRegistry := agent.NewRegistry(database, logger)
	agentRegistrar := agent.NewRegistrar(database, jwtAuth, logger)
	workflowManager := workflow.NewManager(database, logger)
	campaignManager := campaign.NewManager(database, logger)

	// Initialize audit logger (optional)
	var auditLogger *audit.Logger
	if viper.GetBool("quickwit.enabled") {
		quickwitConfig := audit.DefaultQuickwitConfig()
		quickwitConfig.BaseURL = viper.GetString("quickwit.url")
		quickwitConfig.IndexID = viper.GetString("quickwit.index_id")

		quickwitClient := audit.NewQuickwitClient(quickwitConfig, logger)
		auditLogger = audit.NewLogger(quickwitClient, quickwitConfig, logger)

		// Ensure index exists
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := auditLogger.EnsureIndex(ctx); err != nil {
			logger.Warn("failed to ensure audit index", zap.Error(err))
		}
		cancel()
	}

	// Initialize server
	serverConfig := api.DefaultServerConfig()
	serverConfig.Host = viper.GetString("server.host")
	serverConfig.Port = viper.GetInt("server.port")
	serverConfig.Debug = viper.GetBool("server.debug")

	if serverConfig.Host == "" {
		serverConfig.Host = "0.0.0.0"
	}
	if serverConfig.Port == 0 {
		serverConfig.Port = 8080
	}

	server := api.NewServer(serverConfig, &api.Dependencies{
		DB:              database,
		Logger:          logger,
		JWTAuth:         jwtAuth,
		TenantManager:   tenantManager,
		AgentRegistry:   agentRegistry,
		AgentRegistrar:  agentRegistrar,
		WorkflowManager: workflowManager,
		CampaignManager: campaignManager,
		AuditLogger:     auditLogger,
	})

	// Handle shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("received shutdown signal")
		cancel()

		if err := server.Shutdown(context.Background()); err != nil {
			logger.Error("failed to shutdown server", zap.Error(err))
		}

		if auditLogger != nil {
			if err := auditLogger.Close(); err != nil {
				logger.Error("failed to close audit logger", zap.Error(err))
			}
		}
	}()

	// Start server
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return nil
	}
}

func runMCP() error {
	// Initialize logger (to stderr so stdout is for MCP)
	logger, err := createMCPLogger()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Sync()

	// Initialize database
	dbConfig := &db.Config{
		Host:     viper.GetString("database.host"),
		Port:     viper.GetInt("database.port"),
		User:     viper.GetString("database.user"),
		Password: viper.GetString("database.password"),
		Database: viper.GetString("database.name"),
	}

	if dbConfig.Host == "" {
		dbConfig.Host = "localhost"
	}
	if dbConfig.Port == 0 {
		dbConfig.Port = 3306
	}
	if dbConfig.User == "" {
		dbConfig.User = "root"
	}
	if dbConfig.Database == "" {
		dbConfig.Database = "vmmanager"
	}

	database, err := db.NewConnection(dbConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize managers
	agentRegistry := agent.NewRegistry(database, logger)
	workflowManager := workflow.NewManager(database, logger)
	campaignManager := campaign.NewManager(database, logger)

	// Initialize audit logger (optional)
	var auditLogger *audit.Logger
	if viper.GetBool("quickwit.enabled") {
		quickwitConfig := audit.DefaultQuickwitConfig()
		quickwitConfig.BaseURL = viper.GetString("quickwit.url")
		quickwitClient := audit.NewQuickwitClient(quickwitConfig, logger)
		auditLogger = audit.NewLogger(quickwitClient, quickwitConfig, logger)
	}

	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.ServerConfig{
		DB:              database,
		Logger:          logger,
		AgentRegistry:   agentRegistry,
		WorkflowManager: workflowManager,
		CampaignManager: campaignManager,
		AuditLogger:     auditLogger,
	})

	// Handle shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("received shutdown signal")
		cancel()
	}()

	return mcpServer.Run(ctx)
}

func runMigrations() error {
	logger, err := createLogger()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Sync()

	dbConfig := &db.Config{
		Host:     viper.GetString("database.host"),
		Port:     viper.GetInt("database.port"),
		User:     viper.GetString("database.user"),
		Password: viper.GetString("database.password"),
		Database: viper.GetString("database.name"),
	}

	if dbConfig.Host == "" {
		dbConfig.Host = "localhost"
	}
	if dbConfig.Port == 0 {
		dbConfig.Port = 3306
	}
	if dbConfig.User == "" {
		dbConfig.User = "root"
	}
	if dbConfig.Database == "" {
		dbConfig.Database = "vmmanager"
	}

	database, err := db.NewConnection(dbConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	return db.RunMigrations(database, logger)
}

func createLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()

	if viper.GetBool("logging.development") {
		config = zap.NewDevelopmentConfig()
	}

	level := viper.GetString("logging.level")
	if level != "" {
		var zapLevel zapcore.Level
		if err := zapLevel.UnmarshalText([]byte(level)); err == nil {
			config.Level.SetLevel(zapLevel)
		}
	}

	return config.Build()
}

func createMCPLogger() (*zap.Logger, error) {
	// For MCP, log to stderr only
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stderr"}
	config.ErrorOutputPaths = []string{"stderr"}
	return config.Build()
}
