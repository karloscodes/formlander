package internal

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/karloscodes/cartridge"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"formlander/internal/accounts"
	"formlander/internal/config"
	"formlander/internal/database"
	"formlander/internal/forms"
	httphandlers "formlander/internal/http"
	"formlander/internal/jobs"
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

	// The session check runs per request, after NewApp has returned, so it
	// can use the database of the app.
	var app *cartridge.App
	app, err := cartridge.NewApp(cfg,
		cartridge.WithAssets(web.Templates, web.Static),
		// One write connection and a pool of read-only ones. Submissions
		// then cannot fail with "database is locked" against each other,
		// and the admin pages do not wait for a write.
		cartridge.WithReadPool(),
		cartridge.WithServerConfig(func(c *cartridge.ServerConfig) {
			// kamal-proxy (or the operator's TLS proxy) reaches the app from
			// a private or loopback address and appends the visitor's address
			// to X-Forwarded-For. The rate limits key on that address.
			c.ProxyHeader = "X-Forwarded-For"
			c.TrustedProxies = []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}
		}),
		cartridge.WithTemplateFuncs(server.TemplateFuncs()),
		cartridge.WithErrorHandler(server.ErrorHandler(slog.Default(), cfg)),
		cartridge.WithSession("/admin/login"),
		cartridge.WithSessionCheck(func(userID uint, issuedAt time.Time) bool {
			return accounts.SessionValid(app.DBManager.GetConnection(), userID, issuedAt)
		}),
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

	if err := forms.MigrateInlineWebhooks(app.Logger, db); err != nil {
		return fmt.Errorf("migrate inline webhooks: %w", err)
	}

	if err := httphandlers.UpgradeOldStarterHTML(app.Logger, db); err != nil {
		app.Logger.Warn("failed to upgrade old starter templates", slog.Any("error", err))
	}

	// Read connections that were open during the migration would plan
	// their next query with the old schema once.
	app.DBManager.SchemaChanged()

	if err := app.DBManager.CheckpointWAL("FULL"); err != nil {
		app.Logger.Warn("failed to checkpoint WAL after migration", slog.Any("error", err))
	}

	return nil
}

// ensureAdminUser creates the admin on an empty install. Outside the test
// environment, the admin gets a random password, and an install that still
// uses the old public default password gets a new random one.
func ensureAdminUser(db *gorm.DB, cfg *config.Config, logger *slog.Logger) error {
	var count int64
	if err := db.Model(&accounts.User{}).Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		return createAdminUser(db, cfg, logger)
	}
	if !cfg.IsTest() && accounts.IsDefaultAdminActive(db) {
		return replaceDefaultPassword(db, cfg, logger)
	}
	return nil
}

func createAdminUser(db *gorm.DB, cfg *config.Config, logger *slog.Logger) error {
	password := accounts.DefaultAdminPassword
	if !cfg.IsTest() {
		password = accounts.GenerateInitialPassword()
		// Write the file before the database, so the operator never has a
		// password that is stored nowhere.
		if err := accounts.WriteInitialPassword(cfg.DataDirectory, password); err != nil {
			return fmt.Errorf("write initial admin password: %w", err)
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	admin := &accounts.User{
		Email:        accounts.DefaultAdminEmail,
		PasswordHash: string(hash),
	}
	if cfg.IsTest() {
		now := time.Now()
		admin.LastLoginAt = &now
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(admin).Error
	}); err != nil {
		logger.Error("failed to create default admin user", slog.Any("error", err))
		return err
	}

	if !cfg.IsTest() {
		printInitialPassword(cfg, "Admin user created")
	}
	return nil
}

func replaceDefaultPassword(db *gorm.DB, cfg *config.Config, logger *slog.Logger) error {
	password := accounts.GenerateInitialPassword()
	if err := accounts.WriteInitialPassword(cfg.DataDirectory, password); err != nil {
		return fmt.Errorf("write initial admin password: %w", err)
	}

	// ResetPassword also ends every session opened with the public password.
	if err := accounts.ResetPassword(logger, db, accounts.DefaultAdminEmail, password); err != nil {
		logger.Error("failed to replace default admin password", slog.Any("error", err))
		return err
	}

	logger.Warn("replaced the public default admin password with a random one")
	printInitialPassword(cfg, "Default admin password replaced (the old one is public)")
	return nil
}

// printInitialPassword tells where the first admin password is. It does not
// print the password: container logs are kept, and often sent to other
// services, and the password stays valid until the owner changes it.
func printInitialPassword(cfg *config.Config, title string) {
	fmt.Printf("\n🔐 %s:\n", title)
	fmt.Printf("   Email:    %s\n", accounts.DefaultAdminEmail)
	fmt.Printf("   Password: in %s\n", filepath.Join(cfg.DataDirectory, accounts.InitialPasswordFile))
	fmt.Printf("   Change it in Settings after you sign in.\n\n")
}

// GetDB returns the database instance.
func (a *App) GetDB() *gorm.DB {
	db, _ := a.DBManager.Connect()
	return db
}
