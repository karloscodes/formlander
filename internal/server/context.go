// Package server provides formlander-specific server infrastructure that wraps
// the shared github.com/karloscodes/cartridge framework with concrete types.
package server

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"formlander/internal/config"
	"formlander/internal/database"
)

// Context provides request-scoped access to application dependencies.
// This is a formlander-specific context that provides concrete types
// instead of the interfaces used by the generic cartridge package.
type Context struct {
	*fiber.Ctx                   // All Fiber HTTP methods (Render, JSON, etc.)
	Logger    *slog.Logger       // Request logger
	Config    *config.Config     // Runtime configuration
	DBManager *database.Manager  // Database connection pool
	db        *gorm.DB           // Cached database session (lazy-loaded)
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
		panic("server: database connection failed: " + err.Error())
	}

	// Attach the request context for cancellation support and cache it
	ctx.db = db.WithContext(ctx.Context())
	return ctx.db
}

// HandlerFunc is the signature for formlander request handlers.
type HandlerFunc func(*Context) error
