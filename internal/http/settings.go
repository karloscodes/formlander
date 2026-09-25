package http

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"formlander/internal/accounts"
)

// AdminSettingsPage renders the settings page.
func AdminSettingsPage(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Get current user
	userID, ok := GetSession(ctx).GetUserID(ctx.Ctx)
	if !ok {
		return fiber.ErrUnauthorized
	}

	user, err := accounts.FindByID(db, userID)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	// Base template data
	data := fiber.Map{
		"Title":       "Settings",
		"ContentView": "admin/settings/content",
		"User":        user,
	}

	return ctx.Render("layouts/base", data, "")
}

// AdminSettingsUpdatePassword handles password updates from settings page.
func AdminSettingsUpdatePassword(ctx *cartridge.Context) error {
	currentPassword := ctx.FormValue("current_password")
	newPassword := ctx.FormValue("new_password")
	confirmPassword := ctx.FormValue("confirm_password")

	if currentPassword == "" || newPassword == "" || confirmPassword == "" {
		return renderSettingsError(ctx, "All password fields are required")
	}

	if newPassword != confirmPassword {
		return renderSettingsError(ctx, "New passwords do not match")
	}

	userID, ok := GetSession(ctx).GetUserID(ctx.Ctx)
	if !ok {
		return fiber.ErrUnauthorized
	}

	db := ctx.DB()

	user, err := accounts.FindByID(db, userID)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if err := accounts.ChangePassword(ctx.Logger, db, user.Email, currentPassword, newPassword); err != nil {
		if errors.Is(err, accounts.ErrWeakPassword) {
			return renderSettingsError(ctx, "Password must be at least 8 characters long")
		}
		if errors.Is(err, accounts.ErrPasswordMismatch) {
			return renderSettingsError(ctx, "Current password is incorrect")
		}
		ctx.Logger.Error("password change failed in settings", slog.Any("error", err))
		return fiber.ErrInternalServerError
	}

	// The change ends every session, including this one. Start a new one so
	// the person who changed the password stays signed in.
	if err := GetSession(ctx).SetSession(ctx.Ctx, user.ID); err != nil {
		ctx.Logger.Error("failed to renew session after password change", slog.Any("error", err))
	}

	if err := accounts.RemoveInitialPassword(GetAppConfig(ctx).DataDirectory); err != nil {
		ctx.Logger.Warn("failed to remove initial admin password file", slog.Any("error", err))
	}

	return renderSettingsSuccess(ctx, "Password updated successfully")
}

// AdminSettingsUpdateEmail handles email updates from the settings page.
func AdminSettingsUpdateEmail(ctx *cartridge.Context) error {
	newEmail := ctx.FormValue("new_email")
	currentPassword := ctx.FormValue("current_password_email")

	if newEmail == "" || currentPassword == "" {
		return renderSettingsError(ctx, "Email and current password are required")
	}

	userID, ok := GetSession(ctx).GetUserID(ctx.Ctx)
	if !ok {
		return fiber.ErrUnauthorized
	}

	db := ctx.DB()

	user, err := accounts.FindByID(db, userID)
	if err != nil {
		return fiber.ErrInternalServerError
	}

	if err := accounts.ChangeEmail(ctx.Logger, db, user.Email, newEmail, currentPassword); err != nil {
		if errors.Is(err, accounts.ErrInvalidEmail) {
			return renderSettingsError(ctx, "Please enter a valid email address")
		}
		if errors.Is(err, accounts.ErrPasswordMismatch) {
			return renderSettingsError(ctx, "Current password is incorrect")
		}
		if errors.Is(err, accounts.ErrDuplicateEmail) {
			return renderSettingsError(ctx, "That email is already in use")
		}
		ctx.Logger.Error("email change failed in settings", slog.Any("error", err))
		return fiber.ErrInternalServerError
	}

	return renderSettingsSuccess(ctx, "Email updated successfully")
}

// AdminSettingsUpdateMailgun is deprecated - redirect to mailers.
func AdminSettingsUpdateMailgun(ctx *cartridge.Context) error {
	return ctx.Redirect("/admin/settings/mailers")
}

// AdminSettingsUpdateTurnstile is deprecated - redirect to captcha.
func AdminSettingsUpdateTurnstile(ctx *cartridge.Context) error {
	return ctx.Redirect("/admin/settings/captcha")
}

func renderSettingsError(ctx *cartridge.Context, message string) error {
	db := ctx.DB()
	userID, _ := GetSession(ctx).GetUserID(ctx.Ctx)

	user, _ := accounts.FindByID(db, userID)

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Settings",
		"Error":       message,
		"ContentView": "admin/settings/content",
		"User":        user,
	}, "")
}

func renderSettingsSuccess(ctx *cartridge.Context, message string) error {
	db := ctx.DB()
	userID, _ := GetSession(ctx).GetUserID(ctx.Ctx)

	user, _ := accounts.FindByID(db, userID)

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Settings",
		"Success":     message,
		"ContentView": "admin/settings/content",
		"User":        user,
	}, "")
}
