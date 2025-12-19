package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"log/slog"

	"formlander/internal/accounts"
	"formlander/internal/auth"
	"formlander/internal/pkg/cartridge"
)

// RequirePasswordChanged is a middleware that redirects users to change password page if needed.
// Note: This is currently not actively used since the password change redirect happens at login time.
// The redirect to /admin/change-password occurs in AdminLoginSubmit when LastLoginAt is nil (first login).
func RequirePasswordChanged() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// This middleware is kept for compatibility but password change enforcement
		// happens at login time based on LastLoginAt being nil
		return c.Next()
	}
}

// AdminLoginPage renders the admin login form.
func AdminLoginPage(ctx *cartridge.Context) error {
	return ctx.Render("layouts/base", fiber.Map{
		"Title":             "Sign in",
		"HideHeaderActions": true,
		"ContentView":       "admin/login/content",
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

	if err := auth.SetAuthCookie(ctx.Ctx, result.User.ID); err != nil {
		ctx.Logger.Error("failed to set session cookie", slog.Any("error", err), slog.Uint64("userID", uint64(result.User.ID)))
		return fiber.ErrInternalServerError
	}

	// If first login, redirect to password change page
	if result.IsFirstLogin {
		return ctx.Redirect("/admin/change-password")
	}

	return ctx.Redirect("/admin")
}

// AdminLogout destroys the session and redirects to login.
func AdminLogout(ctx *cartridge.Context) error {
	auth.ClearAuthCookie(ctx.Ctx)
	return ctx.Redirect("/admin/login")
}

func renderLoginError(ctx *cartridge.Context, message string) error {
	return ctx.Render("layouts/base", fiber.Map{
		"Title":             "Sign in",
		"Error":             message,
		"HideHeaderActions": true,
		"ContentView":       "admin/login/content",
	}, "")
}

// AdminChangePasswordPage renders the password change form.
func AdminChangePasswordPage(ctx *cartridge.Context) error {
	return ctx.Render("layouts/base", fiber.Map{
		"Title":             "Change Password",
		"HideHeaderActions": true,
		"ContentView":       "admin/change-password/content",
	}, "")
}

// AdminChangePasswordSubmit handles password change requests.
func AdminChangePasswordSubmit(ctx *cartridge.Context) error {
	currentPassword := ctx.FormValue("current_password")
	newPassword := ctx.FormValue("new_password")
	confirmPassword := ctx.FormValue("confirm_password")

	if currentPassword == "" || newPassword == "" || confirmPassword == "" {
		return renderChangePasswordError(ctx, "All fields are required")
	}

	if newPassword != confirmPassword {
		return renderChangePasswordError(ctx, "New passwords do not match")
	}

	userID, ok := auth.GetUserID(ctx.Ctx)
	if !ok {
		return fiber.ErrUnauthorized
	}

	db := ctx.DB()

	user, err := accounts.FindByID(db, userID)
	if err != nil {
		ctx.Logger.Error("failed to find user", slog.Any("error", err), slog.Uint64("userID", uint64(userID)))
		return fiber.ErrUnauthorized
	}

	if err := accounts.ChangePassword(ctx.Logger, db, user.Email, currentPassword, newPassword); err != nil {
		if errors.Is(err, accounts.ErrWeakPassword) {
			return renderChangePasswordError(ctx, "Password must be at least 8 characters long")
		}
		if errors.Is(err, accounts.ErrPasswordMismatch) {
			return renderChangePasswordError(ctx, "Current password is incorrect")
		}
		ctx.Logger.Error("password change failed", slog.Any("error", err))
		return fiber.ErrInternalServerError
	}

	// Redirect to dashboard after successful password change
	return ctx.Redirect("/admin")
}

func renderChangePasswordError(ctx *cartridge.Context, message string) error {
	return ctx.Render("layouts/base", fiber.Map{
		"Title":             "Change Password",
		"Error":             message,
		"HideHeaderActions": true,
		"ContentView":       "admin/change-password/content",
	}, "")
}
