package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"formlander/internal/config"
)

// GetAppConfig retrieves the formlander config from context locals.
func GetAppConfig(ctx *cartridge.Context) *config.Config {
	return ctx.Ctx.Locals("app_config").(*config.Config)
}

// GetAppConfigFromFiber retrieves the formlander config from fiber context locals.
func GetAppConfigFromFiber(c *fiber.Ctx) *config.Config {
	return c.Locals("app_config").(*config.Config)
}

// GetSession retrieves the session manager from context locals.
func GetSession(ctx *cartridge.Context) *cartridge.SessionManager {
	return ctx.Ctx.Locals("session").(*cartridge.SessionManager)
}

// GetSessionFromFiber retrieves the session manager from fiber context locals.
func GetSessionFromFiber(c *fiber.Ctx) *cartridge.SessionManager {
	return c.Locals("session").(*cartridge.SessionManager)
}
