package http

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"formlander/internal/accounts"
)

// AdminLoginPage renders the admin login form.
func AdminLoginPage(ctx *cartridge.Context) error {
	return ctx.Render("layouts/base", fiber.Map{
		"Title":                  "Sign in",
		"HideHeaderActions":      true,
		"ContentView":            "admin/login/content",
		"ShowDefaultCredentials": accounts.IsDefaultAdminActive(ctx.DB()),
	}, "")
}

// AdminLoginSubmit handles credential verification.
func AdminLoginSubmit(ctx *cartridge.Context) error {
	email := ctx.FormValue("email")
	password := ctx.FormValue("password")

	db := ctx.DB()

	result, err := accounts.Authenticate(ctx.Logger, db, email, password)
	if err != nil {
		if errors.Is(err, accounts.ErrInvalidCredentials) || errors.Is(err, accounts.ErrMissingFields) {
			return renderLoginError(ctx, "Invalid credentials")
		}
		ctx.Logger.Error("authentication failed", slog.Any("error", err))
		return fiber.ErrInternalServerError
	}

	if err := GetSession(ctx).SetSession(ctx.Ctx, result.User.ID); err != nil {
		ctx.Logger.Error("failed to set session cookie", slog.Any("error", err), slog.Uint64("userID", uint64(result.User.ID)))
		return fiber.ErrInternalServerError
	}

	// The default credentials keep working until the operator changes the
	// password themselves (in settings) — no forced first-login change.
	return ctx.Redirect("/admin")
}

// AdminLogout destroys the session and redirects to login.
func AdminLogout(ctx *cartridge.Context) error {
	GetSession(ctx).ClearSession(ctx.Ctx)
	return ctx.Redirect("/admin/login")
}

func renderLoginError(ctx *cartridge.Context, message string) error {
	return ctx.Render("layouts/base", fiber.Map{
		"Title":                  "Sign in",
		"Error":                  message,
		"HideHeaderActions":      true,
		"ContentView":            "admin/login/content",
		"ShowDefaultCredentials": accounts.IsDefaultAdminActive(ctx.DB()),
	}, "")
}

// Password changes are handled in the settings page (AdminSettingsUpdatePassword).
