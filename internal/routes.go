package internal

import (
	"net/http"
	"time"

	"github.com/karloscodes/cartridge"
	ratelimit "github.com/karloscodes/cartridge/middleware"

	"formlander/internal/config"
	httphandlers "formlander/internal/http"
)

// MountRoutes registers all application routes.
func MountRoutes(s *cartridge.Server, cfg *config.Config) {
	// Store formlander config and session in all requests for handlers
	s.Use(func(ctx *cartridge.Context) error {
		ctx.Locals("app_config", cfg)
		return ctx.Next()
	})

	// Health Check - support both GET and HEAD requests
	healthHandler := func(ctx *cartridge.Context) error {
		return ctx.Status(http.StatusOK).JSON(cartridge.Map{"status": "ok"})
	}
	s.Get("/_health", healthHandler)
	s.Head("/_health", healthHandler)

	s.Get("/", func(ctx *cartridge.Context) error {
		return ctx.Redirect("/admin")
	})

	// Public demo page
	s.Get("/_demo", httphandlers.DemoContactForm)

	// Build middleware chain for public routes (rate limiting disabled in dev/test)
	publicMiddleware := []cartridge.HandlerFunc{
		ratelimit.RateLimiter(ratelimit.WithMax(30), ratelimit.WithDuration(time.Minute), ratelimit.WithEnv(cfg)),
	}

	publicConfig := &cartridge.RouteConfig{
		EnableSecFetchSite: cartridge.Bool(false), // Public APIs accept cross-origin requests
		EnableCORS:         true,
		CORSConfig: &cartridge.CORSConfig{
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
		return ctx.SendStatus(http.StatusNoContent)
	}, publicConfig)

	s.Get("/admin/login", httphandlers.AdminLoginPage)

	// Rate limit login attempts: 5 per minute per IP (disabled in dev/test mode)
	loginRateLimiter := ratelimit.RateLimiter(
		ratelimit.WithMax(5),
		ratelimit.WithDuration(time.Minute),
		ratelimit.WithEnv(cfg),
		ratelimit.WithLimitReached(func(ctx *cartridge.Context) error {
			return ctx.Render("layouts/base", cartridge.Map{
				"Title":             "Sign in",
				"Error":             "Too many login attempts. Please try again in a minute.",
				"HideHeaderActions": true,
				"ContentView":       "admin/login/content",
			}, "")
		}),
	)

	// Disable Sec-Fetch-Site enforcement on login: cartridge's strict
	// middleware rejects requests missing the header (older browsers,
	// reverse proxies that strip fetch-metadata), which locked users
	// out of fresh deployments. CSRF on an unauthenticated login form
	// is low-risk — the attacker gains nothing by forcing a victim to
	// submit credentials they don't already control.
	s.Post("/admin/login", httphandlers.AdminLoginSubmit, &cartridge.RouteConfig{
		EnableSecFetchSite: cartridge.Bool(false),
		CustomMiddleware:   []cartridge.HandlerFunc{loginRateLimiter},
	})

	// Auth config for protected routes: a valid session.
	authConfig := &cartridge.RouteConfig{
		CustomMiddleware: []cartridge.HandlerFunc{s.Session().Middleware()},
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
	s.Get("/admin/submissions/export.csv", httphandlers.SubmissionsExport, authConfig)
	s.Get("/admin/submissions/:id", httphandlers.AdminSubmissionShow, authConfig)
	s.Get("/admin/submissions/:id/files/:file_id", httphandlers.AdminSubmissionFileDownload, authConfig)
	s.Post("/admin/submissions/delete", httphandlers.AdminSubmissionsDelete, authConfig)
	s.Post("/admin/submissions/:id/delete", httphandlers.AdminSubmissionDelete, authConfig)

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
	s.Post("/admin/settings/mailers/:id/test", httphandlers.MailerProfileTest, authConfig)

	// Captcha Profile routes
	s.Get("/admin/settings/captcha", httphandlers.CaptchaProfileList, authConfig)
	s.Get("/admin/settings/captcha/new", httphandlers.CaptchaProfileNew, authConfig)
	s.Post("/admin/settings/captcha", httphandlers.CaptchaProfileCreate, authConfig)
	s.Get("/admin/settings/captcha/:id", httphandlers.CaptchaProfileShow, authConfig)
	s.Get("/admin/settings/captcha/:id/edit", httphandlers.CaptchaProfileEdit, authConfig)
	s.Post("/admin/settings/captcha/:id", httphandlers.CaptchaProfileUpdate, authConfig)
	s.Post("/admin/settings/captcha/:id/delete", httphandlers.CaptchaProfileDelete, authConfig)
	s.Post("/admin/settings/captcha/:id/test", httphandlers.CaptchaProfileTest, authConfig)

	// Webhook Profile routes
	s.Get("/admin/settings/webhooks", httphandlers.WebhookProfileList, authConfig)
	s.Get("/admin/settings/webhooks/new", httphandlers.WebhookProfileNew, authConfig)
	s.Post("/admin/settings/webhooks", httphandlers.WebhookProfileCreate, authConfig)
	s.Get("/admin/settings/webhooks/:id", httphandlers.WebhookProfileShow, authConfig)
	s.Get("/admin/settings/webhooks/:id/edit", httphandlers.WebhookProfileEdit, authConfig)
	s.Post("/admin/settings/webhooks/:id", httphandlers.WebhookProfileUpdate, authConfig)
	s.Post("/admin/settings/webhooks/:id/delete", httphandlers.WebhookProfileDelete, authConfig)
	s.Post("/admin/settings/webhooks/:id/test", httphandlers.WebhookProfileTest, authConfig)

	// Submissions routes
	s.Get("/admin/submissions", httphandlers.SubmissionList, authConfig)
}
