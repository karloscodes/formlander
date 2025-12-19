package internal

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/karloscodes/cartridge"

	"formlander/internal/config"
	httphandlers "formlander/internal/http"
	"formlander/internal/middleware"
)

// MountRoutes registers all application routes.
func MountRoutes(s *cartridge.Server, cfg *config.Config) {
	// Store formlander config and session in all requests for handlers
	s.App().Use(func(c *fiber.Ctx) error {
		c.Locals("app_config", cfg)
		c.Locals("session", s.Session())
		return c.Next()
	})

	// Health Check - support both GET and HEAD requests
	healthHandler := func(ctx *cartridge.Context) error {
		return ctx.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	}
	s.Get("/_health", healthHandler)
	s.App().Head("/_health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	s.Get("/", func(ctx *cartridge.Context) error {
		return ctx.Redirect("/admin")
	})

	// Public demo page
	s.Get("/_demo", httphandlers.DemoContactForm)

	// Build middleware chain for public routes (rate limiting disabled in dev/test)
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
				// Skip rate limiting in dev/test mode
				return cfg.IsDevelopment() || cfg.IsTest()
			},
		}),
	}

	publicConfig := &cartridge.RouteConfig{
		EnableSecFetchSite: cartridge.Bool(false), // Public APIs accept cross-origin requests
		EnableCORS:         true,
		CORSConfig: &cors.Config{
			AllowOrigins: "*",
			AllowMethods: "POST,OPTIONS",
			AllowHeaders: "Content-Type, Authorization, User-Agent",
		},
		WriteConcurrency: true,
		CustomMiddleware: publicMiddleware,
	}

	s.Post("/forms/:slug/submit", httphandlers.PublicFormSubmission, publicConfig)
	s.Options("/forms/:slug/submit", func(ctx *cartridge.Context) error {
		return ctx.SendStatus(fiber.StatusNoContent)
	}, publicConfig)

	s.Post("/x/api/v1/submissions", httphandlers.APISubmissionCreate, publicConfig)
	s.Options("/x/api/v1/submissions", func(ctx *cartridge.Context) error {
		return ctx.SendStatus(fiber.StatusNoContent)
	}, publicConfig)

	s.Get("/admin/login", httphandlers.AdminLoginPage)

	// Rate limit login attempts: 5 per minute per IP (disabled in dev/test mode)
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
			// Skip rate limiting in dev/test mode
			return cfg.IsDevelopment() || cfg.IsTest()
		},
	})

	s.Post("/admin/login", httphandlers.AdminLoginSubmit, &cartridge.RouteConfig{
		CustomMiddleware: []fiber.Handler{loginRateLimiter},
	})

	// Auth config without password check (for change-password routes)
	authConfigBasic := &cartridge.RouteConfig{
		CustomMiddleware: []fiber.Handler{s.Session().Middleware()},
	}

	// Auth config with password change enforcement (for protected routes)
	authConfig := &cartridge.RouteConfig{
		CustomMiddleware: []fiber.Handler{s.Session().Middleware(), httphandlers.RequirePasswordChanged()},
	}

	// Password change routes (accessible to authenticated users)
	// Note: First login password change is enforced at login time via LastLoginAt check
	s.Get("/admin/change-password", httphandlers.AdminChangePasswordPage, authConfigBasic)
	s.Post("/admin/change-password", httphandlers.AdminChangePasswordSubmit, authConfigBasic)

	// Protected routes that require password to be changed
	s.Get("/admin", httphandlers.AdminDashboard, authConfig)
	s.Post("/admin/logout", httphandlers.AdminLogout, authConfig)
	s.Get("/admin/forms", httphandlers.AdminFormsIndex, authConfig)
	s.Get("/admin/forms/new", httphandlers.AdminFormsNew, authConfig)
	s.Post("/admin/forms", httphandlers.AdminFormsCreate, authConfig)
	s.Get("/admin/forms/:id", httphandlers.AdminFormShow, authConfig)
	s.Get("/admin/forms/:id/edit", httphandlers.AdminFormsEdit, authConfig)
	s.Post("/admin/forms/:id", httphandlers.AdminFormsUpdate, authConfig)
	s.Get("/admin/submissions/:id", httphandlers.AdminSubmissionShow, authConfig)

	// Pro feature paywall pages

	// Settings routes
	s.Get("/admin/settings", httphandlers.AdminSettingsPage, authConfig)
	s.Post("/admin/settings/password", httphandlers.AdminSettingsUpdatePassword, authConfig)
	s.Post("/admin/settings/mailgun", httphandlers.AdminSettingsUpdateMailgun, authConfig)
	s.Post("/admin/settings/turnstile", httphandlers.AdminSettingsUpdateTurnstile, authConfig)

	// Mailer Profile routes
	s.Get("/admin/settings/mailers", httphandlers.MailerProfileList, authConfig)
	s.Get("/admin/settings/mailers/new", httphandlers.MailerProfileNew, authConfig)
	s.Post("/admin/settings/mailers", httphandlers.MailerProfileCreate, authConfig)
	s.Get("/admin/settings/mailers/:id", httphandlers.MailerProfileShow, authConfig)
	s.Get("/admin/settings/mailers/:id/edit", httphandlers.MailerProfileEdit, authConfig)
	s.Post("/admin/settings/mailers/:id", httphandlers.MailerProfileUpdate, authConfig)
	s.Post("/admin/settings/mailers/:id/delete", httphandlers.MailerProfileDelete, authConfig)

	// Captcha Profile routes
	s.Get("/admin/settings/captcha", httphandlers.CaptchaProfileList, authConfig)
	s.Get("/admin/settings/captcha/new", httphandlers.CaptchaProfileNew, authConfig)
	s.Post("/admin/settings/captcha", httphandlers.CaptchaProfileCreate, authConfig)
	s.Get("/admin/settings/captcha/:id", httphandlers.CaptchaProfileShow, authConfig)
	s.Get("/admin/settings/captcha/:id/edit", httphandlers.CaptchaProfileEdit, authConfig)
	s.Post("/admin/settings/captcha/:id", httphandlers.CaptchaProfileUpdate, authConfig)
	s.Post("/admin/settings/captcha/:id/delete", httphandlers.CaptchaProfileDelete, authConfig)

	// Submissions routes
	s.Get("/admin/submissions", httphandlers.SubmissionList, authConfig)
}
