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
	"formlander/internal/accounts"
	"formlander/internal/database"

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
		printInitialPassword(m)
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
	case "reset-admin-password":
		// Runs inside the container; change-admin-password calls it there.
		if err := runResetAdminPassword(os.Args[2:]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "change-admin-password":
		if err := runAdminPasswordChange(m); err != nil {
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

// printInitialPassword shows the first admin password that the app wrote to
// its storage volume on first boot. A reinstall over existing data has none.
func printInitialPassword(m *matcha.Matcha) {
	path := filepath.Join(m.DataDir(), "storage", accounts.InitialPasswordFile)
	password, err := os.ReadFile(path)
	if err != nil {
		return
	}
	fmt.Println("Sign in with:")
	fmt.Printf("  Email:    %s\n", accounts.DefaultAdminEmail)
	fmt.Printf("  Password: %s\n", strings.TrimSpace(string(password)))
	fmt.Println("Change the password in Settings after you sign in.")
}

// runResetAdminPassword sets the admin password directly in the database.
func runResetAdminPassword(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: formlander reset-admin-password <email> <new-password>")
	}

	app, err := internal.NewApp()
	if err != nil {
		return err
	}
	if err := accounts.ResetPassword(app.Logger, app.GetDB(), args[0], args[1]); err != nil {
		return err
	}
	// The first password no longer works, so its file must go too.
	return accounts.RemoveInitialPassword(app.Config.DataDirectory)
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
	if err := m.Exec("/usr/local/bin/formlander", "reset-admin-password", email, password); err != nil {
		return fmt.Errorf("failed to change password: %w", err)
	}

	fmt.Println("Password changed successfully.")
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
	fmt.Println("Other Commands:")
	fmt.Println("  version                     Show version info")
	fmt.Println("  help                        Show this help")
}
