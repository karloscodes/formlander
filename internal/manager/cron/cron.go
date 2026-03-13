package cron

import (
	"fmt"
	"os"
	"path/filepath"

	"formlander/internal/manager/logging"
	"formlander/internal/manager/product"
)

var productSettings = product.Get()

var (
	// DefaultCronFile is the path to the cron job file
	DefaultCronFile = productSettings.CronFile
	// DefaultInstallDir is the default installation directory
	DefaultInstallDir = productSettings.InstallDir
	// DefaultBinaryPath is the path to the installer binary
	DefaultBinaryPath = productSettings.BinaryPath
	// DefaultCronSchedule is the default schedule for the cron job (3:00 AM daily)
	DefaultCronSchedule = productSettings.CronSchedule
)

// Manager handles cron job operations
type Manager struct {
	logger     *logging.Logger
	cronFile   string
	installDir string
	binaryPath string
	schedule   string
}

// NewManager creates a new cron manager with default settings
func NewManager(logger *logging.Logger) *Manager {
	return &Manager{
		logger:     logger,
		cronFile:   DefaultCronFile,
		installDir: DefaultInstallDir,
		binaryPath: DefaultBinaryPath,
		schedule:   DefaultCronSchedule,
	}
}

// SetupCronJob creates or updates the cron job for automated updates
func (m *Manager) SetupCronJob() error {
	if os.Getenv("ENV") == "test" {
		m.logger.InfoWithTime("Skipping cron setup in test environment")
		return nil
	}

	// Create a more robust cron job with better environment setup
	cronContent := fmt.Sprintf("# %s automated updates\n", productSettings.ProductName)
	cronContent += "SHELL=/bin/bash\n"
	cronContent += "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n"
	cronContent += fmt.Sprintf("INSTALL_DIR=%s\n", m.installDir)
	cronContent += fmt.Sprintf("%s root cd %s && %s update > %s/logs/updater.log 2>&1\n",
		m.schedule,
		m.installDir,
		m.binaryPath,
		m.installDir)

	m.logger.Info("Setting up cron job...")

	// Ensure the logs directory exists
	logsDir := filepath.Join(m.installDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		m.logger.Warn("Failed to create logs directory: %v", err)
	}

	if err := os.WriteFile(m.cronFile, []byte(cronContent), 0o644); err != nil {
		m.logger.Error("Cron setup failed: %v", err)
		return fmt.Errorf("failed to write cron file %s: %w", m.cronFile, err)
	}

	m.logger.Success("Cron job setup complete")
	m.logger.InfoWithTime("Automatic updates scheduled for 3:00 AM daily")
	return nil
}
