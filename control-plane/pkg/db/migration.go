// Package db provides database connectivity for the control plane.
package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Migration represents a database migration
type Migration struct {
	Version string
	Name    string
	SQL     string
}

// MigrationRunner runs database migrations
type MigrationRunner struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewMigrationRunner creates a new migration runner
func NewMigrationRunner(db *gorm.DB, logger *zap.Logger) *MigrationRunner {
	return &MigrationRunner{
		db:     db,
		logger: logger,
	}
}

// migrationHistory tracks applied migrations
type migrationHistory struct {
	Version   string `gorm:"primaryKey;size:64"`
	AppliedAt string `gorm:"not null"`
}

func (migrationHistory) TableName() string {
	return "schema_migrations"
}

// Run executes all pending migrations from a directory
func (r *MigrationRunner) Run(migrationsDir string) error {
	// Ensure migrations table exists
	if err := r.db.AutoMigrate(&migrationHistory{}); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get applied migrations
	applied, err := r.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Read migration files
	migrations, err := r.readMigrationFiles(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migration files: %w", err)
	}

	// Run pending migrations
	for _, migration := range migrations {
		if applied[migration.Version] {
			continue
		}

		r.logger.Info("applying migration",
			zap.String("version", migration.Version),
			zap.String("name", migration.Name))

		if err := r.applyMigration(migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}

		r.logger.Info("migration applied successfully",
			zap.String("version", migration.Version))
	}

	return nil
}

// getAppliedMigrations returns a map of applied migration versions
func (r *MigrationRunner) getAppliedMigrations() (map[string]bool, error) {
	var history []migrationHistory
	if err := r.db.Find(&history).Error; err != nil {
		return nil, err
	}

	applied := make(map[string]bool)
	for _, h := range history {
		applied[h.Version] = true
	}

	return applied, nil
}

// readMigrationFiles reads all migration files from a directory
func (r *MigrationRunner) readMigrationFiles(dir string) ([]Migration, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []Migration
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", file.Name(), err)
		}

		// Parse version from filename (e.g., "001_initial.sql")
		name := strings.TrimSuffix(file.Name(), ".sql")
		parts := strings.SplitN(name, "_", 2)
		version := parts[0]
		migrationName := name
		if len(parts) > 1 {
			migrationName = parts[1]
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    migrationName,
			SQL:     string(content),
		})
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// applyMigration applies a single migration
func (r *MigrationRunner) applyMigration(migration Migration) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Execute the migration SQL
		if err := tx.Exec(migration.SQL).Error; err != nil {
			return fmt.Errorf("failed to execute SQL: %w", err)
		}

		// Record the migration
		history := migrationHistory{
			Version:   migration.Version,
			AppliedAt: "NOW()",
		}
		if err := tx.Create(&history).Error; err != nil {
			return fmt.Errorf("failed to record migration: %w", err)
		}

		return nil
	})
}

// Rollback rolls back the last n migrations
func (r *MigrationRunner) Rollback(n int) error {
	// Get applied migrations in reverse order
	var history []migrationHistory
	if err := r.db.Order("version DESC").Limit(n).Find(&history).Error; err != nil {
		return fmt.Errorf("failed to get migration history: %w", err)
	}

	for _, h := range history {
		r.logger.Info("rolling back migration",
			zap.String("version", h.Version))

		if err := r.db.Delete(&h).Error; err != nil {
			return fmt.Errorf("failed to delete migration record: %w", err)
		}
	}

	return nil
}

// Status returns the current migration status
func (r *MigrationRunner) Status() ([]migrationHistory, error) {
	var history []migrationHistory
	if err := r.db.Order("version ASC").Find(&history).Error; err != nil {
		return nil, err
	}
	return history, nil
}
