package controllers

import (
	"fmt"
	"formlander/internal/forms"
	"formlander/internal/integrations"
	"strconv"

	"formlander/internal/pkg/cartridge"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// MailerProfileList shows all mailer profiles.
func MailerProfileList(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	var profiles []integrations.MailerProfile
	if err := db.Order("created_at DESC").Find(&profiles).Error; err != nil {
		return fiber.ErrInternalServerError
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Mailer Profiles",
		"Profiles":    profiles,
		"ContentView": "admin/mailers/index",
	}, "")
}

// MailerProfileNew shows the create form.
func MailerProfileNew(ctx *cartridge.Context) error {
	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "New Mailer Profile",
		"ContentView": "admin/mailers/new/content",
	}, "")
}

// MailerProfileCreate handles profile creation.
func MailerProfileCreate(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	logger := ctx.Logger
	params := integrations.MailerProfileParams{
		Name:             ctx.FormValue("name"),
		Provider:         ctx.FormValue("provider"),
		APIKey:           ctx.FormValue("api_key"),
		Domain:           ctx.FormValue("domain"),
		DefaultFromName:  ctx.FormValue("default_from_name"),
		DefaultFromEmail: ctx.FormValue("default_from_email"),
		DefaultsJSON:     ctx.FormValue("defaults_json"),
	}

	_, err = integrations.CreateMailerProfile(logger, db, params)
	if err != nil {
		var errMsg string
		if valErr, ok := err.(*integrations.ValidationError); ok {
			errMsg = valErr.Message
		} else {
			errMsg = err.Error()
		}
		return ctx.Render("layouts/base", fiber.Map{
			"Title":       "New Mailer Profile",
			"Error":       errMsg,
			"ContentView": "admin/mailers/new/content",
		}, "")
	}

	return ctx.Redirect("/admin/settings/mailers")
}

// MailerProfileShow displays a single profile.
func MailerProfileShow(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	id := ctx.Params("id")
	var profile integrations.MailerProfile
	if err := db.First(&profile, id).Error; err != nil {
		return fiber.ErrNotFound
	}

	// Count email deliveries using this profile
	var usageCount int64
	db.Model(&forms.EmailDelivery{}).Where("mailer_profile_id = ?", profile.ID).Count(&usageCount)

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Mailer Profile: " + profile.Name,
		"Profile":     profile,
		"UsageCount":  usageCount,
		"ContentView": "admin/mailers/show/content",
	}, "")
}

// MailerProfileEdit shows the edit form.
func MailerProfileEdit(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	id := ctx.Params("id")
	var profile integrations.MailerProfile
	if err := db.First(&profile, id).Error; err != nil {
		return fiber.ErrNotFound
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Edit Mailer Profile",
		"Profile":     profile,
		"IsEdit":      true,
		"ContentView": "admin/mailers/new/content",
	}, "")
}

// MailerProfileUpdate handles profile updates.
func MailerProfileUpdate(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	id := ctx.Params("id")
	profileID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return fiber.ErrNotFound
	}

	logger := ctx.Logger
	params := integrations.MailerProfileParams{
		Name:             ctx.FormValue("name"),
		Provider:         ctx.FormValue("provider"),
		APIKey:           ctx.FormValue("api_key"),
		Domain:           ctx.FormValue("domain"),
		DefaultFromName:  ctx.FormValue("default_from_name"),
		DefaultFromEmail: ctx.FormValue("default_from_email"),
		DefaultsJSON:     ctx.FormValue("defaults_json"),
	}

	profile, err := integrations.UpdateMailerProfile(logger, db, uint(profileID), params)
	if err != nil {
		// Get profile for error display
		existingProfile, _ := integrations.GetMailerProfileByID(db, uint(profileID))
		var errMsg string
		if valErr, ok := err.(*integrations.ValidationError); ok {
			errMsg = valErr.Message
		} else {
			errMsg = err.Error()
		}
		return ctx.Render("layouts/base", fiber.Map{
			"Title":       "Edit Mailer Profile",
			"Profile":     existingProfile,
			"Error":       errMsg,
			"IsEdit":      true,
			"ContentView": "admin/mailers/new/content",
		}, "")
	}

	return ctx.Redirect("/admin/settings/mailers/" + fmt.Sprint(profile.ID))
}

// MailerProfileDelete removes a profile.
func MailerProfileDelete(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	id := ctx.Params("id")
	profileID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return fiber.ErrNotFound
	}

	// Check if any email deliveries are using this profile
	var count int64
	db.Model(&forms.EmailDelivery{}).Where("mailer_profile_id = ?", profileID).Count(&count)
	if count > 0 {
		return ctx.Status(400).SendString("Cannot delete profile: it is being used by forms")
	}

	logger := ctx.Logger
	if err := integrations.DeleteMailerProfile(logger, db, uint(profileID)); err != nil {
		logger.Error("failed to delete mailer profile", zap.Error(err), zap.Uint("profile_id", uint(profileID)))
		return fiber.ErrInternalServerError
	}

	return ctx.Redirect("/admin/settings/mailers")
}
