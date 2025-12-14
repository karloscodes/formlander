package controllers

import (
	"encoding/json"
	"fmt"
	"formlander/internal/forms"
	"formlander/internal/integrations"
	"strconv"

	"formlander/internal/pkg/cartridge"

	"github.com/gofiber/fiber/v2"
	"log/slog"
)

type siteKeyEntry struct {
	HostPattern string `json:"host_pattern"`
	SiteKey     string `json:"site_key"`
}

// CaptchaProfileList shows all captcha profiles.
func CaptchaProfileList(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	var profiles []integrations.CaptchaProfile
	if err := db.Order("created_at DESC").Find(&profiles).Error; err != nil {
		return fiber.ErrInternalServerError
	}

	// Add site key count to each profile
	type profileWithCount struct {
		integrations.CaptchaProfile
		SiteKeyCount int
	}
	var profilesWithCount []profileWithCount
	for _, p := range profiles {
		var siteKeys []siteKeyEntry
		count := 0
		if p.SiteKeysJSON != "" {
			if err := json.Unmarshal([]byte(p.SiteKeysJSON), &siteKeys); err == nil {
				count = len(siteKeys)
			}
		}
		profilesWithCount = append(profilesWithCount, profileWithCount{
			CaptchaProfile: p,
			SiteKeyCount:   count,
		})
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Captcha Profiles",
		"Profiles":    profilesWithCount,
		"ContentView": "admin/captcha/index/content",
	}, "")
}

// CaptchaProfileNew shows the create form.
func CaptchaProfileNew(ctx *cartridge.Context) error {
	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "New Captcha Profile",
		"ContentView": "admin/captcha/new/content",
	}, "")
}

// CaptchaProfileCreate handles profile creation.
func CaptchaProfileCreate(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	logger := ctx.Logger
	params := integrations.CaptchaProfileParams{
		Name:         ctx.FormValue("name"),
		Provider:     ctx.FormValue("provider"),
		SecretKey:    ctx.FormValue("secret_key"),
		SiteKeysJSON: ctx.FormValue("site_keys_json"),
		PolicyJSON:   ctx.FormValue("policy_json"),
	}

	_, err = integrations.CreateCaptchaProfile(logger, db, params)
	if err != nil {
		var errMsg string
		if valErr, ok := err.(*integrations.ValidationError); ok {
			errMsg = valErr.Message
		} else {
			errMsg = err.Error()
		}
		return ctx.Render("layouts/base", fiber.Map{
			"Title":       "New Captcha Profile",
			"Error":       errMsg,
			"ContentView": "admin/captcha/new/content",
		}, "")
	}

	return ctx.Redirect("/admin/settings/captcha")
}

// CaptchaProfileShow displays a single profile.
func CaptchaProfileShow(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	id := ctx.Params("id")
	var profile integrations.CaptchaProfile
	if err := db.First(&profile, id).Error; err != nil {
		return fiber.ErrNotFound
	}

	// Parse site keys for display
	var siteKeys []siteKeyEntry
	if profile.SiteKeysJSON != "" {
		json.Unmarshal([]byte(profile.SiteKeysJSON), &siteKeys)
	}

	// Count forms using this profile
	var usageCount int64
	db.Model(&forms.Form{}).Where("captcha_profile_id = ?", profile.ID).Count(&usageCount)

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Captcha Profile: " + profile.Name,
		"Profile":     profile,
		"SiteKeys":    siteKeys,
		"UsageCount":  usageCount,
		"ContentView": "admin/captcha/show/content",
	}, "")
}

// CaptchaProfileEdit shows the edit form.
func CaptchaProfileEdit(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	id := ctx.Params("id")
	var profile integrations.CaptchaProfile
	if err := db.First(&profile, id).Error; err != nil {
		return fiber.ErrNotFound
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Edit Captcha Profile",
		"Profile":     profile,
		"IsEdit":      true,
		"ContentView": "admin/captcha/new/content",
	}, "")
}

// CaptchaProfileUpdate handles profile updates.
func CaptchaProfileUpdate(ctx *cartridge.Context) error {
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
	params := integrations.CaptchaProfileParams{
		Name:         ctx.FormValue("name"),
		Provider:     ctx.FormValue("provider"),
		SecretKey:    ctx.FormValue("secret_key"),
		SiteKeysJSON: ctx.FormValue("site_keys_json"),
		PolicyJSON:   ctx.FormValue("policy_json"),
	}

	profile, err := integrations.UpdateCaptchaProfile(logger, db, uint(profileID), params)
	if err != nil {
		// Get profile for error display
		existingProfile, _ := integrations.GetCaptchaProfileByID(db, uint(profileID))
		var errMsg string
		if valErr, ok := err.(*integrations.ValidationError); ok {
			errMsg = valErr.Message
		} else {
			errMsg = err.Error()
		}
		return ctx.Render("layouts/base", fiber.Map{
			"Title":       "Edit Captcha Profile",
			"Profile":     existingProfile,
			"Error":       errMsg,
			"IsEdit":      true,
			"ContentView": "admin/captcha/new/content",
		}, "")
	}

	return ctx.Redirect("/admin/settings/captcha/" + fmt.Sprint(profile.ID))
}

// CaptchaProfileDelete removes a profile.
func CaptchaProfileDelete(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	id := ctx.Params("id")
	profileID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return fiber.ErrNotFound
	}

	// Check if any forms are using this profile
	var count int64
	db.Model(&forms.Form{}).Where("captcha_profile_id = ?", profileID).Count(&count)
	if count > 0 {
		return ctx.Status(400).SendString("Cannot delete profile: it is being used by forms")
	}

	logger := ctx.Logger
	if err := integrations.DeleteCaptchaProfile(logger, db, uint(profileID)); err != nil {
		logger.Error("failed to delete captcha profile", slog.Any("error", err), slog.Uint64("profile_id", uint64(profileID)))
		return fiber.ErrInternalServerError
	}

	return ctx.Redirect("/admin/settings/captcha")
}
