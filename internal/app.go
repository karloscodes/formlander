package internal

import (
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/karloscodes/cartridge"

	"formlander/internal/accounts"
	"formlander/internal/config"
	"formlander/internal/database"
	"formlander/internal/jobs"
	"formlander/internal/pkg/dbtxn"
	"formlander/internal/server"
	"formlander/web"
)

// App wraps the cartridge app with formlander-specific config.
type App struct {
	*cartridge.App
	Config *config.Config
}

// NewApp creates the formlander application.
func NewApp() (*App, error) {
	cfg := config.Get()

	app, err := cartridge.NewSSRApp("formlander",
		cartridge.WithConfig(cfg.Config),
		cartridge.WithAssets(web.Templates, web.Static),
		cartridge.WithTemplateFuncs(server.TemplateFuncs()),
		cartridge.WithErrorHandler(server.ErrorHandler(slog.Default(), cfg)),
		cartridge.WithSession("/admin/login"),
		cartridge.WithJobs(2*time.Minute,
			jobs.NewWebhookDispatcher(cfg),
			jobs.NewEmailDispatcher(cfg),
		),
		cartridge.WithRoutes(func(s *cartridge.Server) {
			MountRoutes(s, cfg)
		}),
	)
	if err != nil {
		return nil, err
	}

	return &App{App: app, Config: cfg}, nil
}

// RunMigrations runs database migrations and ensures admin user exists.
func RunMigrations(app *App) error {
	db, err := app.DBManager.Connect()
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	if err := database.Migrate(db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	if err := ensureAdminUser(db, app.Config, app.Logger); err != nil {
		return fmt.Errorf("ensure admin user: %w", err)
	}

	if err := app.DBManager.CheckpointWAL("FULL"); err != nil {
		app.Logger.Warn("failed to checkpoint WAL after migration", slog.Any("error", err))
	}

	return nil
}

func ensureAdminUser(db *gorm.DB, cfg *config.Config, logger *slog.Logger) error {
	var count int64
	if err := db.Model(&accounts.User{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	defaultEmail := "admin@formlander.local"
	defaultPassword := "formlander"

	hash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	admin := &accounts.User{
		Email:        defaultEmail,
		PasswordHash: string(hash),
		LastLoginAt:  nil,
	}

	if cfg.IsTest() {
		now := time.Now()
		admin.LastLoginAt = &now
	}

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Create(admin).Error
	}); err != nil {
		logger.Error("failed to create default admin user", slog.Any("error", err))
		return err
	}

	fmt.Printf("\n🔐 Default admin user created:\n")
	fmt.Printf("   Email: %s\n", defaultEmail)
	fmt.Printf("   Temporary credentials: %s\n", defaultPassword)
	fmt.Printf("   ⚠️  You will be required to change this on first login\n\n")

	return nil
}

// GetDB returns the database instance.
func (a *App) GetDB() *gorm.DB {
	db, _ := a.DBManager.Connect()
	return db
}
