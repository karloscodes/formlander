package cartridge

import (
	"fmt"
	"log/slog"

	"github.com/gofiber/fiber/v2"
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
// Panics if the database connection fails (caught by recover middleware).
func (ctx *Context) DB() *gorm.DB {
	if ctx.db != nil {
		return ctx.db
	}

	db, err := ctx.DBManager.Connect()
	if err != nil {
		panic(fmt.Errorf("cartridge: database connection failed: %w", err))
	}

	// Attach the request context for cancellation support and cache it
	ctx.db = db.WithContext(ctx.Context())
	return ctx.db
}
