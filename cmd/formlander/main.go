package main

import (
	"flag"
	"log"
	"time"

	"formlander/internal"
	"formlander/internal/database"
)

func main() {
	seedFlag := flag.Bool("seed", false, "Seed the database with sample data")
	flag.Parse()

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

	// Run with graceful shutdown
	if err := app.RunWithTimeout(10 * time.Second); err != nil {
		log.Fatal(err)
	}
}
