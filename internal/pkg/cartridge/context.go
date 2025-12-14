package cartridge

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"log/slog"
	"gorm.io/gorm"

	"formlander/internal/config"
	"formlander/internal/database"
)

// Context provides request-scoped access to application dependencies.
// It embeds fiber.Ctx to provide all HTTP request/response methods while
// adding direct field access to logger, config, and database manager.
// This eliminates the need for context.Locals and provides type-safe access.
type Context struct {
	*fiber.Ctx                   // All Fiber HTTP methods (Render, JSON, etc.)
	Logger     *slog.Logger       // Request logger (shared across app)
	Config     *config.Config    // Runtime configuration
	DBManager  *database.Manager // Database connection pool
	db         *gorm.DB          // Cached database session (lazy-loaded)
}

// DB provides a per-request database session with context attached.
// The connection is cached after first call within the same request.
// Returns an error if the database connection fails.
func (ctx *Context) DB() (*gorm.DB, error) {
	if ctx.db != nil {
		return ctx.db, nil
	}

	db, err := ctx.DBManager.Connect()
	if err != nil {
		ctx.Logger.Error("failed to connect to database", slog.Any("error", err))
		return nil, fmt.Errorf("cartridge: database connection failed: %w", err)
	}

	// Attach the request context for cancellation support and cache it
	ctx.db = db.WithContext(ctx.Context())
	return ctx.db, nil
}
