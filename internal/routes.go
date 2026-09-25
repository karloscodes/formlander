package internal

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/karloscodes/cartridge"

	"formlander/internal/accounts"
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
		limiter.New(limiter.Config{
			Max:        30,
			Expiration: 60 * time.Second,
			KeyGenerator: func(c *fiber.Ctx) string {
				return middleware.ClientIP(c)
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
	s.Get("/forms/sent", httphandlers.SubmissionSent)
	s.Options("/forms/:slug/submit", func(ctx *cartridge.Context) error {
		return ctx.SendStatus(fiber.StatusNoContent)
	}, publicConfig)

	s.Get("/admin/login", httphandlers.AdminLoginPage)

	// Rate limit login attempts: 5 per minute per IP (disabled in dev/test mode)
	loginRateLimiter := limiter.New(limiter.Config{
		Max:        5,
		Expiration: 60 * time.Second,
		KeyGenerator: func(c *fiber.Ctx) string {
			return middleware.ClientIP(c)
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

	// Disable Sec-Fetch-Site enforcement on login: cartridge's strict
	// middleware rejects requests missing the header (older browsers,
	// reverse proxies that strip fetch-metadata), which locked users
	// out of fresh deployments. CSRF on an unauthenticated login form
	// is low-risk — the attacker gains nothing by forcing a victim to
	// submit credentials they don't already control.
	s.Post("/admin/login", httphandlers.AdminLoginSubmit, &cartridge.RouteConfig{
		EnableSecFetchSite: cartridge.Bool(false),
		CustomMiddleware:   []fiber.Handler{loginRateLimiter},
	})

	// Auth config for protected routes: a valid session.
	authConfig := &cartridge.RouteConfig{
		CustomMiddleware: []fiber.Handler{s.Session().Middleware(), endStaleSessions(s)},
	}

	// Protected routes (require a logged-in session).
	s.Get("/admin", httphandlers.AdminDashboard, authConfig)
	s.Post("/admin/logout", httphandlers.AdminLogout, authConfig)
	s.Get("/admin/forms", httphandlers.AdminFormsIndex, authConfig)
	s.Get("/admin/forms/new", httphandlers.AdminFormsNew, authConfig)
	s.Post("/admin/forms", httphandlers.AdminFormsCreate, authConfig)
	s.Get("/admin/forms/:id", httphandlers.AdminFormShow, authConfig)
	s.Get("/admin/forms/:id/edit", httphandlers.AdminFormsEdit, authConfig)
	s.Post("/admin/forms/:id", httphandlers.AdminFormsUpdate, authConfig)
	s.Get("/admin/submissions/:id", httphandlers.AdminSubmissionShow, authConfig)
	s.Get("/admin/submissions/:id/files/:file_id", httphandlers.AdminSubmissionFileDownload, authConfig)

	// Settings routes
	s.Get("/admin/settings", httphandlers.AdminSettingsPage, authConfig)
	s.Post("/admin/settings/password", httphandlers.AdminSettingsUpdatePassword, authConfig)
	s.Post("/admin/settings/email", httphandlers.AdminSettingsUpdateEmail, authConfig)
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

// endStaleSessions signs out sessions issued before the last password
// change, and sessions of users that no longer exist.
func endStaleSessions(s *cartridge.Server) fiber.Handler {
	return func(c *fiber.Ctx) error {
		session := s.Session()
		userID, ok := session.GetUserID(c)
		issuedAt, _ := session.IssuedAt(c)
		if ok {
			user, err := accounts.FindByID(s.GetDBManager().GetConnection(), userID)
			if err == nil && user.SessionIsCurrent(issuedAt) {
				return c.Next()
			}
		}
		session.ClearSession(c)
		return c.Redirect("/admin/login")
	}
}
