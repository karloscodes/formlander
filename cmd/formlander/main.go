package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"formlander/internal"
	"formlander/internal/accounts"
	"formlander/internal/database"
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

	switch os.Args[1] {
	case "serve", "server", "run":
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runServer()
	case "reset-admin-password":
		// Runs inside the container: chasen -a formlander run formlander reset-admin-password <email> <password>
		if err := runResetAdminPassword(os.Args[2:]); err != nil {
			fmt.Printf("Error: %v\n", err)
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
	fmt.Println("Admin Commands (inside the container):")
	fmt.Println("  reset-admin-password <email> <password>")
	fmt.Println("                              Set the admin password")
	fmt.Println("")
	fmt.Println("Other Commands:")
	fmt.Println("  version                     Show version info")
	fmt.Println("  help                        Show this help")
}
