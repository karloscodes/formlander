package http

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"formlander/internal/accounts"
)

// AdminLoginPage renders the admin login form.
func AdminLoginPage(ctx *cartridge.Context) error {
	return ctx.Render("layouts/base", fiber.Map{
		"Title":                   "Sign in",
		"HideHeaderActions":       true,
		"ContentView":             "admin/login/content",
		"ShowInitialPasswordHint": accounts.HasInitialPassword(GetAppConfig(ctx).DataDirectory),
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

	return ctx.Redirect("/admin")
}

// AdminLogout ends the session and redirects to login. The browser deletes its
// cookie, and the server records the session as ended, so a copy of the
// cookie no longer works either. Other sessions of the admin stay signed in.
func AdminLogout(ctx *cartridge.Context) error {
	session := GetSession(ctx)
	if userID, ok := session.GetUserID(ctx.Ctx); ok {
		issuedAt, _ := session.IssuedAt(ctx.Ctx)
		maxAge := time.Duration(GetAppConfig(ctx).SessionTimeout) * time.Second
		if err := accounts.EndSession(ctx.Logger, ctx.DB(), userID, issuedAt, maxAge); err != nil {
			ctx.Logger.Error("failed to record the end of a session", slog.Any("error", err))
		}
	}
	session.ClearSession(ctx.Ctx)
	return ctx.Redirect("/admin/login")
}

func renderLoginError(ctx *cartridge.Context, message string) error {
	return ctx.Render("layouts/base", fiber.Map{
		"Title":                   "Sign in",
		"Error":                   message,
		"HideHeaderActions":       true,
		"ContentView":             "admin/login/content",
		"ShowInitialPasswordHint": accounts.HasInitialPassword(GetAppConfig(ctx).DataDirectory),
	}, "")
}

// Password changes are handled in the settings page (AdminSettingsUpdatePassword).
