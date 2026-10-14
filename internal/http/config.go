package http

import (
	"github.com/karloscodes/cartridge"

	"formlander/internal/config"
)

// GetAppConfig retrieves the formlander config from context locals.
func GetAppConfig(ctx *cartridge.Context) *config.Config {
	return ctx.Locals("app_config").(*config.Config)
}

// GetSession retrieves the session manager of the app.
func GetSession(ctx *cartridge.Context) *cartridge.SessionManager {
	return ctx.Session
}
