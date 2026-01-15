// Package db provides database connectivity for the control plane.
package db

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/yourorg/control-plane/pkg/db/models"
)

// Config contains database configuration
type Config struct {
	Host               string
	Port               int
	Username           string
	Password           string
	Database           string
	MaxConnections     int
	MaxIdleConnections int
	ConnectionLifetime time.Duration
	LogLevel           string
}

// Connection wraps the GORM database connection
type Connection struct {
	db     *gorm.DB
	config *Config
	logger *zap.Logger
}

// NewConnection creates a new database connection
func NewConnection(cfg *Config, zapLogger *zap.Logger) (*Connection, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=UTC",
		cfg.Username,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
	)

	// Configure GORM logger
	var logLevel logger.LogLevel
	switch cfg.LogLevel {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	case "info":
		logLevel = logger.Info
	default:
		logLevel = logger.Warn
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	}

	db, err := gorm.Open(mysql.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxConnections)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConnections)
	sqlDB.SetConnMaxLifetime(cfg.ConnectionLifetime)

	conn := &Connection{
		db:     db,
		config: cfg,
		logger: zapLogger,
	}

	zapLogger.Info("database connection established",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database))

	return conn, nil
}

// DB returns the underlying GORM database instance
func (c *Connection) DB() *gorm.DB {
	return c.db
}

// Close closes the database connection
func (c *Connection) Close() error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Ping checks the database connection
func (c *Connection) Ping() error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// AutoMigrate runs auto-migration for all models
func (c *Connection) AutoMigrate() error {
	return c.db.AutoMigrate(
		&models.Tenant{},
		&models.TenantAPIKey{},
		&models.InstallationKey{},
		&models.Agent{},
		&models.AgentToken{},
		&models.AgentHealthReport{},
		&models.Workflow{},
		&models.WorkflowExecution{},
		&models.Campaign{},
		&models.CampaignPhase{},
	)
}

// WithTenant returns a DB instance scoped to a tenant
func (c *Connection) WithTenant(tenantID string) *gorm.DB {
	return c.db.Where("tenant_id = ?", tenantID)
}

// Transaction executes a function within a transaction
func (c *Connection) Transaction(fn func(tx *gorm.DB) error) error {
	return c.db.Transaction(fn)
}

// Stats returns database connection statistics
type Stats struct {
	MaxOpenConnections int `json:"max_open_connections"`
	OpenConnections    int `json:"open_connections"`
	InUse              int `json:"in_use"`
	Idle               int `json:"idle"`
}

// GetStats returns connection pool statistics
func (c *Connection) GetStats() (*Stats, error) {
	sqlDB, err := c.db.DB()
	if err != nil {
		return nil, err
	}

	stats := sqlDB.Stats()
	return &Stats{
		MaxOpenConnections: stats.MaxOpenConnections,
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
	}, nil
}
