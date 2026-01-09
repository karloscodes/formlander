package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"formlander/internal"
	"formlander/internal/database"
	"formlander/internal/installer/admin"
	"formlander/internal/installer/config"
	installerPkg "formlander/internal/installer/installer"
	"formlander/internal/installer/license"
	"formlander/internal/installer/logging"
	"formlander/internal/installer/product"
	"formlander/internal/installer/updater"
	"formlander/internal/installer/validation"

	"golang.org/x/term"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

var productSettings = product.Get()

func main() {
	// If no args or first arg starts with -, run the web server
	if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
		runServer()
		return
	}

	// Handle subcommands
	switch os.Args[1] {
	case "serve", "server", "run":
		// Remove the subcommand from args so flags work
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runServer()
	case "install":
		runInstall()
	case "update":
		runUpdate()
	case "reload":
		runReload()
	case "upgrade-to-pro":
		runUpgradeToPro()
	case "restore-db":
		runRestoreDB()
	case "change-admin-password":
		runAdminPasswordChange()
	case "update-license-key":
		runUpdateLicenseKey()
	case "version", "--version", "-v":
		printVersion()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// runServer starts the Formlander web application
func runServer() {
	seedFlag := flag.Bool("seed", false, "Seed the database with sample data")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		printVersion()
		return
	}

	app, err := internal.NewApp()
	if err != nil {
		log.Fatal(err)
	}

	// Run database migrations
	log.Println("Running database migrations...")
	if err := internal.RunMigrations(app); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed")

	// If seed flag is provided, seed and exit
	if *seedFlag {
		db := app.GetDB()

		log.Println("Seeding database...")
		if err := database.Seed(db); err != nil {
			log.Fatalf("Failed to seed database: %v", err)
		}
		log.Println("Database seeded successfully!")
		return
	}

	// Run with graceful shutdown (shorter timeout in dev/test)
	shutdownTimeout := 2 * time.Second
	if app.Config.IsProduction() {
		shutdownTimeout = 10 * time.Second
	}
	if err := app.RunWithTimeout(shutdownTimeout); err != nil {
		log.Fatal(err)
	}
}

func initLogger() *logging.Logger {
	logLevel := "info"
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		logLevel = envLevel
	}
	return logging.NewLogger(logging.Config{
		Level:   logLevel,
		Verbose: os.Getenv("VERBOSE") == "true",
		Quiet:   os.Getenv("QUIET") == "true",
	})
}

func runInstall() {
	startTime := time.Now()
	logger := initLogger()
	inst := installerPkg.NewInstaller(logger)

	if err := inst.RunCompleteInstallation(); err != nil {
		logger.Error("Installation failed: %v", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime).Round(time.Second)
	logger.Success("Installation completed in %s", elapsed)
	inst.DisplayCompletionMessage()
}

func runUpdate() {
	startTime := time.Now()
	logger := initLogger()

	u := updater.NewUpdater(logger)
	if err := u.Run(version); err != nil {
		logger.Error("Update failed: %v", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime).Round(time.Second)
	logger.Success("Update completed in %s", elapsed)
}

func runReload() {
	startTime := time.Now()
	logger := initLogger()

	reloader := updater.NewReloader(logger)
	if err := reloader.Run(); err != nil {
		logger.Error("Reload failed: %v", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime).Round(time.Second)
	logger.Success("Reload completed in %s", elapsed)
}

func runUpgradeToPro() {
	startTime := time.Now()
	logger := initLogger()
	reader := bufio.NewReader(os.Stdin)

	envFile := filepath.Join(productSettings.InstallDir, ".env")

	// Check if .env file exists (installation required first)
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		logger.Error(".env file not found at %s", envFile)
		logger.Error("Please run 'formlander install' first")
		os.Exit(1)
	}

	// Load current configuration
	cfg := config.NewConfig(logger)
	if err := cfg.LoadFromFile(envFile); err != nil {
		logger.Error("Failed to load configuration: %v", err)
		os.Exit(1)
	}

	// Check if already on Pro
	data := cfg.GetData()
	if strings.Contains(data.AppImage, "formlander-pro") {
		logger.Info("Already running Formlander Pro")
		os.Exit(0)
	}

	fmt.Println("")
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║           Upgrade to Formlander Pro                        ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println("")
	fmt.Println("Pro features include:")
	fmt.Println("  • AI-powered form builder")
	fmt.Println("  • Email autoresponders")
	fmt.Println("  • Priority support")
	fmt.Println("")
	fmt.Println("Purchase at: https://formlander.com/pro")
	fmt.Println("")

	// Prompt for license key
	fmt.Print("Enter your Gumroad license key: ")
	licenseKeyInput, err := reader.ReadString('\n')
	if err != nil {
		logger.Error("Failed to read license key: %v", err)
		os.Exit(1)
	}
	licenseKey := strings.TrimSpace(licenseKeyInput)

	if licenseKey == "" {
		logger.Error("License key cannot be empty")
		os.Exit(1)
	}

	// Validate license key format
	if err := validation.ValidateLicenseKey(licenseKey); err != nil {
		logger.Error("Invalid license key format: %v", err)
		os.Exit(1)
	}

	// Validate with Gumroad
	logger.Info("Validating license with Gumroad...")
	email, err := license.Validate(licenseKey)
	if err != nil {
		logger.Error("License validation failed: %v", err)
		os.Exit(1)
	}
	logger.Success("License valid for: %s", email)

	// Update configuration
	data.AppImage = productSettings.ProAppImage
	data.LicenseKey = licenseKey
	cfg.SetData(data)

	if err := cfg.SaveToFile(envFile); err != nil {
		logger.Error("Failed to save configuration: %v", err)
		os.Exit(1)
	}

	logger.Info("Configuration updated to use Pro image")

	// Reload containers
	logger.Info("Reloading containers with Pro version...")
	reloader := updater.NewReloader(logger)
	if err := reloader.Run(); err != nil {
		logger.Error("Failed to reload containers: %v", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime).Round(time.Second)
	fmt.Println("")
	logger.Success("Upgraded to Formlander Pro in %s", elapsed)
	fmt.Println("")
	fmt.Println("Pro features are now available!")
	fmt.Println("Visit your dashboard to try the AI Form Builder.")
}

func runRestoreDB() {
	startTime := time.Now()
	logger := initLogger()
	inst := installerPkg.NewInstaller(logger)
	reader := bufio.NewReader(os.Stdin)

	backups, err := inst.ListBackups()
	if err != nil {
		logger.Error("Failed to list backups: %v", err)
		os.Exit(1)
	}

	if len(backups) == 0 {
		logger.Error("No backups found in %s", inst.GetBackupDir())
		os.Exit(1)
	}

	selectedBackup, err := inst.PromptBackupSelection(backups)
	if err != nil {
		logger.Error("Backup selection failed: %v", err)
		os.Exit(1)
	}

	if err := inst.ValidateBackup(selectedBackup); err != nil {
		logger.Error("Backup validation failed: %v", err)
		os.Exit(1)
	}

	fmt.Printf("⚠️  This will replace your current database.\n")
	fmt.Printf("   Selected backup: %s\n", selectedBackup)
	fmt.Print("Are you sure? (yes/no): ")

	confirm, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(confirm)) != "yes" {
		logger.Info("Restore cancelled")
		os.Exit(0)
	}

	if err := inst.RestoreFromBackup(selectedBackup); err != nil {
		logger.Error("Restore failed: %v", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime).Round(time.Second)
	logger.Success("Database restored in %s", elapsed)
}

func runAdminPasswordChange() {
	logger := initLogger()
	adminMgr := admin.NewManager(logger)
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter admin email: ")
	emailInput, _ := reader.ReadString('\n')
	email := strings.TrimSpace(emailInput)

	if err := validation.ValidateEmail(email); err != nil {
		logger.Error("Invalid email: %v", err)
		os.Exit(1)
	}

	fmt.Print("Enter new password (min 8 chars): ")
	passBytes, _ := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	password := strings.TrimSpace(string(passBytes))

	if err := validation.ValidatePassword(password); err != nil {
		logger.Error("Invalid password: %v", err)
		os.Exit(1)
	}

	if err := adminMgr.ChangeAdminPassword(email, password); err != nil {
		logger.Error("Failed to change password: %v", err)
		os.Exit(1)
	}

	logger.Success("Password changed successfully")
}

func runUpdateLicenseKey() {
	startTime := time.Now()
	logger := initLogger()
	envFile := filepath.Join(productSettings.InstallDir, ".env")

	var licenseKey string
	if len(os.Args) >= 3 {
		licenseKey = os.Args[2]
	} else {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter license key: ")
		input, _ := reader.ReadString('\n')
		licenseKey = strings.TrimSpace(input)
	}

	if licenseKey == "" {
		logger.Error("License key cannot be empty")
		os.Exit(1)
	}

	if err := validation.ValidateLicenseKey(licenseKey); err != nil {
		logger.Error("Invalid license key: %v", err)
		os.Exit(1)
	}

	cfg := config.NewConfig(logger)
	if err := cfg.LoadFromFile(envFile); err != nil {
		logger.Error("Failed to load config: %v", err)
		os.Exit(1)
	}

	data := cfg.GetData()
	data.LicenseKey = licenseKey
	cfg.SetData(data)

	if err := cfg.SaveToFile(envFile); err != nil {
		logger.Error("Failed to save config: %v", err)
		os.Exit(1)
	}

	logger.Info("Reloading containers...")
	reloader := updater.NewReloader(logger)
	if err := reloader.Run(); err != nil {
		logger.Error("Reload failed: %v", err)
		os.Exit(1)
	}

	elapsed := time.Since(startTime).Round(time.Second)
	logger.Success("License key updated in %s", elapsed)
}

func printVersion() {
	fmt.Printf("Formlander %s\n", version)
	fmt.Printf("  Commit:     %s\n", commit)
	fmt.Printf("  Build Time: %s\n", buildTime)
}

func printUsage() {
	fmt.Println("Formlander - Self-hosted form backend")
	fmt.Println("")
	fmt.Println("Usage: formlander [command] [options]")
	fmt.Println("")
	fmt.Println("Server Commands:")
	fmt.Println("  serve                       Start the web server (default)")
	fmt.Println("  --seed                      Seed database with sample data")
	fmt.Println("")
	fmt.Println("Installer Commands:")
	fmt.Println("  install                     Install Formlander via Docker")
	fmt.Println("  update                      Update existing installation")
	fmt.Println("  reload                      Reload containers")
	fmt.Println("  upgrade-to-pro              Upgrade to Formlander Pro")
	fmt.Println("  restore-db                  Restore database from backup")
	fmt.Println("  change-admin-password       Change admin password")
	fmt.Println("  update-license-key [key]    Update license key")
	fmt.Println("")
	fmt.Println("Other Commands:")
	fmt.Println("  version                     Show version info")
	fmt.Println("  help                        Show this help")
}
