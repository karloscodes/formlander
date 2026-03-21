package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"time"

	"formlander/internal"
	"formlander/internal/database"
	"formlander/internal/license"

	"github.com/karloscodes/matcha"
	"golang.org/x/term"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
		runServer()
		return
	}

	m := newMatcha()

	switch os.Args[1] {
	case "serve", "server", "run":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runServer()
	case "install":
		if err := m.Install(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "update":
		if err := m.Update(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "reload":
		if err := m.Reload(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "restore-db":
		if err := m.RestoreDB(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "change-admin-password":
		if err := runAdminPasswordChange(m); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "upgrade-to-pro":
		runUpgradeToPro(m)
	case "update-license-key":
		if err := runUpdateLicenseKey(m); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "check":
		if err := matcha.Check(); err != nil {
			os.Exit(1)
		}
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

func newMatcha() *matcha.Matcha {
	return matcha.New(matcha.Config{
		Name:           "formlander",
		AppImage:       "karloscodes/formlander:latest",
		HealthPath:     "/_health",
		Volumes:        []string{"/app/storage", "/app/logs"},
		CronUpdates:    true,
		Backups:        true,
		ManagerRepo:    "karloscodes/formlander",
		ManagerVersion: version,
	})
}

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

	log.Println("Running database migrations...")
	if err := internal.RunMigrations(app); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed")

	if *seedFlag {
		db := app.GetDB()
		log.Println("Seeding database...")
		if err := database.Seed(db); err != nil {
			log.Fatalf("Failed to seed database: %v", err)
		}
		log.Println("Database seeded successfully!")
		return
	}

	shutdownTimeout := 2 * time.Second
	if app.Config.IsProduction() {
		shutdownTimeout = 10 * time.Second
	}
	if err := app.RunWithTimeout(shutdownTimeout); err != nil {
		log.Fatal(err)
	}
}

func runAdminPasswordChange(m *matcha.Matcha) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter admin email: ")
	email, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read email: %w", err)
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	var password string
	for {
		fmt.Print("Enter new admin password (minimum 8 characters): ")
		passBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println()

		password = strings.TrimSpace(string(passBytes))
		if len(password) < 8 {
			fmt.Println("Error: password must be at least 8 characters")
			continue
		}

		fmt.Print("Confirm new admin password: ")
		confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println()

		if password != strings.TrimSpace(string(confirmBytes)) {
			fmt.Println("Error: Passwords do not match. Please try again.")
			continue
		}
		break
	}

	fmt.Println("Changing password...")
	if err := m.Exec("/app/fnctl", "change-admin-password", email, password); err != nil {
		return fmt.Errorf("failed to change password: %w", err)
	}

	fmt.Println("Password changed successfully.")
	return nil
}

const proImage = "karloscodes/formlander-pro:latest"

func runUpgradeToPro(m *matcha.Matcha) {
	fmt.Println("Upgrade to Formlander Pro")
	fmt.Println("========================")
	fmt.Println()
	fmt.Println("This will:")
	fmt.Println("  - Back up your current database")
	fmt.Println("  - Validate your license key")
	fmt.Println("  - Switch to the Pro Docker image")
	fmt.Println("  - Restart containers")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter your Gumroad license key: ")
	licenseKeyInput, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	licenseKey := strings.TrimSpace(licenseKeyInput)

	if licenseKey == "" {
		fmt.Println("Error: License key cannot be empty")
		os.Exit(1)
	}

	fmt.Println("Validating license with Gumroad...")
	email, err := license.Validate(licenseKey)
	if err != nil {
		fmt.Printf("Error: License validation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("License valid for: %s\n", email)

	fmt.Println("Backing up database...")
	if _, err := m.BackupDB(); err != nil {
		fmt.Printf("Warning: backup failed: %v\n", err)
		fmt.Println("Proceeding without backup...")
	}

	fmt.Println("Switching to Formlander Pro...")
	m.SetImage(proImage)

	if err := m.SaveImage(); err != nil {
		fmt.Printf("Error: failed to save image config: %v\n", err)
		os.Exit(1)
	}

	if err := m.Deploy(); err != nil {
		fmt.Printf("Error: upgrade failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Upgrade completed successfully!")
	if domain, err := m.GetDomain(); err == nil && domain != "" {
		fmt.Printf("Visit https://%s to complete Pro setup\n", domain)
	}
}

func runUpdateLicenseKey(m *matcha.Matcha) error {
	var licenseKey string
	if len(os.Args) >= 3 {
		licenseKey = os.Args[2]
	} else {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter license key: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read license key: %w", err)
		}
		licenseKey = strings.TrimSpace(input)
	}

	if licenseKey == "" {
		return fmt.Errorf("license key cannot be empty")
	}

	app, err := matcha.LoadApp("formlander")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if app.Env == nil {
		app.Env = make(map[string]string)
	}
	app.Env["FORMLANDER_LICENSE_KEY"] = licenseKey

	if err := matcha.SaveApp("formlander", app); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println("Reloading containers with new license key...")
	if err := m.Reload(); err != nil {
		return fmt.Errorf("failed to reload: %w", err)
	}

	fmt.Println("License key updated successfully.")
	return nil
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
	fmt.Println("  restore-db                  Restore database from backup")
	fmt.Println("  change-admin-password       Change admin password")
	fmt.Println("  check                       Check server security")
	fmt.Println("")
	fmt.Println("Pro Commands:")
	fmt.Println("  upgrade-to-pro              Upgrade to Formlander Pro")
	fmt.Println("  update-license-key [key]    Update license key")
	fmt.Println("")
	fmt.Println("Other Commands:")
	fmt.Println("  version                     Show version info")
	fmt.Println("  help                        Show this help")
}
