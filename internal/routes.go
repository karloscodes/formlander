package internal

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"formlander/internal/auth"
	httphandlers "formlander/internal/http"
	"formlander/internal/pkg/cartridge"
	"formlander/internal/pkg/cartridge/middleware"
)

// MountRoutes registers all application routes.
func MountRoutes(server *cartridge.Server) {
	// Health Check - support both GET and HEAD requests
	healthHandler := func(ctx *cartridge.Context) error {
		return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	}
	server.Get("/_health", healthHandler)
	server.App().Head("/_health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	server.Get("/", func(ctx *cartridge.Context) error {
		return ctx.Redirect("/admin")
	})

	// Public demo page
	server.Get("/_demo", httphandlers.DemoContactForm)

	// Build middleware chain for public routes
	publicMiddleware := []fiber.Handler{
		middleware.TurnstileMiddleware(),
		limiter.New(limiter.Config{
			Max:        30,
			Expiration: 60 * time.Second,
			KeyGenerator: func(c *fiber.Ctx) string {
				return c.IP()
			},
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error": "rate limit exceeded",
				})
			},
			Next: func(c *fiber.Ctx) bool {
				// Skip rate limiting in test mode
				ctx, ok := c.Locals("cartridge_ctx").(*cartridge.Context)
				if ok && ctx.Config != nil && ctx.Config.IsTest() {
					return true
				}
				return false
			},
		}),
	}

	publicConfig := &cartridge.RouteConfig{
		EnableCORS: true,
		CORSConfig: &cors.Config{
			AllowOrigins: "*",
			AllowMethods: "POST,OPTIONS",
			AllowHeaders: "Content-Type, Authorization, User-Agent",
		},
		WriteConcurrency: true,
		CustomMiddleware: publicMiddleware,
	}

	server.Post("/forms/:slug/submit", httphandlers.PublicFormSubmission, publicConfig)
	server.Options("/forms/:slug/submit", func(ctx *cartridge.Context) error {
		return ctx.SendStatus(fiber.StatusNoContent)
	}, publicConfig)

	server.Post("/x/api/v1/submissions", httphandlers.APISubmissionCreate, publicConfig)
	server.Options("/x/api/v1/submissions", func(ctx *cartridge.Context) error {
		return ctx.SendStatus(fiber.StatusNoContent)
	}, publicConfig)

	server.Get("/admin/login", httphandlers.AdminLoginPage)
	
	// Rate limit login attempts: 5 per minute per IP (disabled in test mode)
	loginRateLimiter := limiter.New(limiter.Config{
		Max:        5,
		Expiration: 60 * time.Second,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).Render("layouts/base", fiber.Map{
				"Title":             "Sign in",
				"Error":             "Too many login attempts. Please try again in a minute.",
				"HideHeaderActions": true,
				"ContentView":       "admin/login/content",
			}, "")
		},
		Next: func(c *fiber.Ctx) bool {
			// Skip rate limiting in test mode
			ctx, ok := c.Locals("cartridge_ctx").(*cartridge.Context)
			if ok && ctx.Config != nil && ctx.Config.IsTest() {
				return true
			}
			return false
		},
	})
	
	server.Post("/admin/login", httphandlers.AdminLoginSubmit, &cartridge.RouteConfig{
		CustomMiddleware: []fiber.Handler{loginRateLimiter},
	})

	// Auth config without password check (for change-password routes)
	authConfigBasic := &cartridge.RouteConfig{
		CustomMiddleware: []fiber.Handler{auth.Middleware()},
	}

	// Auth config with password change enforcement (for protected routes)
	authConfig := &cartridge.RouteConfig{
		CustomMiddleware: []fiber.Handler{auth.Middleware(), httphandlers.RequirePasswordChanged()},
	}

	// Password change routes (accessible to authenticated users)
	// Note: First login password change is enforced at login time via LastLoginAt check
	server.Get("/admin/change-password", httphandlers.AdminChangePasswordPage, authConfigBasic)
	server.Post("/admin/change-password", httphandlers.AdminChangePasswordSubmit, authConfigBasic)

	// Protected routes that require password to be changed
	server.Get("/admin", httphandlers.AdminDashboard, authConfig)
	server.Post("/admin/logout", httphandlers.AdminLogout, authConfig)
	server.Get("/admin/forms", httphandlers.AdminFormsIndex, authConfig)
	server.Get("/admin/forms/new", httphandlers.AdminFormsNew, authConfig)
	server.Post("/admin/forms", httphandlers.AdminFormsCreate, authConfig)
	server.Get("/admin/forms/:id", httphandlers.AdminFormShow, authConfig)
	server.Get("/admin/forms/:id/edit", httphandlers.AdminFormsEdit, authConfig)
	server.Post("/admin/forms/:id", httphandlers.AdminFormsUpdate, authConfig)
	server.Get("/admin/submissions/:id", httphandlers.AdminSubmissionShow, authConfig)
	
	// Pro feature paywall pages

	// Settings routes
	server.Get("/admin/settings", httphandlers.AdminSettingsPage, authConfig)
	server.Post("/admin/settings/password", httphandlers.AdminSettingsUpdatePassword, authConfig)
	server.Post("/admin/settings/mailgun", httphandlers.AdminSettingsUpdateMailgun, authConfig)
	server.Post("/admin/settings/turnstile", httphandlers.AdminSettingsUpdateTurnstile, authConfig)

	// Mailer Profile routes
	server.Get("/admin/settings/mailers", httphandlers.MailerProfileList, authConfig)
	server.Get("/admin/settings/mailers/new", httphandlers.MailerProfileNew, authConfig)
	server.Post("/admin/settings/mailers", httphandlers.MailerProfileCreate, authConfig)
	server.Get("/admin/settings/mailers/:id", httphandlers.MailerProfileShow, authConfig)
	server.Get("/admin/settings/mailers/:id/edit", httphandlers.MailerProfileEdit, authConfig)
	server.Post("/admin/settings/mailers/:id", httphandlers.MailerProfileUpdate, authConfig)
	server.Post("/admin/settings/mailers/:id/delete", httphandlers.MailerProfileDelete, authConfig)

	// Captcha Profile routes
	server.Get("/admin/settings/captcha", httphandlers.CaptchaProfileList, authConfig)
	server.Get("/admin/settings/captcha/new", httphandlers.CaptchaProfileNew, authConfig)
	server.Post("/admin/settings/captcha", httphandlers.CaptchaProfileCreate, authConfig)
	server.Get("/admin/settings/captcha/:id", httphandlers.CaptchaProfileShow, authConfig)
	server.Get("/admin/settings/captcha/:id/edit", httphandlers.CaptchaProfileEdit, authConfig)
	server.Post("/admin/settings/captcha/:id", httphandlers.CaptchaProfileUpdate, authConfig)
	server.Post("/admin/settings/captcha/:id/delete", httphandlers.CaptchaProfileDelete, authConfig)

	// Submissions routes
	server.Get("/admin/submissions", httphandlers.SubmissionList, authConfig)
}
