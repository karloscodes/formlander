package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"formlander/internal/integrations"
	"formlander/internal/middleware"
)

type siteKeyEntry struct {
	HostPattern string `json:"host_pattern"`
	SiteKey     string `json:"site_key"`
}

// siteKeysFromForm returns the site keys of a profile as the JSON the model
// keeps. The form sends one site_key and one host_pattern for each row: the
// first row is the Site Key field, the others are the rows under "One key
// for each domain". A key without a domain covers every site.
//
// An older version of the form sent site_keys_json. It still wins when a
// request has it.
func siteKeysFromForm(ctx *cartridge.Context) string {
	if raw := strings.TrimSpace(ctx.FormValue("site_keys_json")); raw != "" {
		return raw
	}
	hosts := postedValues(ctx, "host_pattern")
	var entries []siteKeyEntry
	for i, siteKey := range postedValues(ctx, "site_key") {
		siteKey = strings.TrimSpace(siteKey)
		if siteKey == "" {
			continue
		}
		host := ""
		if i < len(hosts) {
			host = strings.TrimSpace(hosts[i])
		}
		if host == "" {
			host = "*"
		}
		entries = append(entries, siteKeyEntry{HostPattern: host, SiteKey: siteKey})
	}
	if len(entries) == 0 {
		return ""
	}
	data, _ := json.Marshal(entries)
	return string(data)
}

// policyFromForm returns the widget options of a profile as the JSON the
// model keeps: the theme, the size, the language, and the action. An option
// left at its default is not saved. saved is the policy the profile has now:
// options that the form does not have stay.
//
// An older version of the form sent policy_json. It still wins when a
// request has it.
func policyFromForm(ctx *cartridge.Context, saved string) string {
	if raw := strings.TrimSpace(ctx.FormValue("policy_json")); raw != "" {
		return raw
	}
	policy := map[string]any{}
	_ = json.Unmarshal([]byte(saved), &policy)
	for _, key := range []string{"theme", "size", "language", "action"} {
		delete(policy, key)
	}
	if theme := ctx.FormValue("theme"); theme == "light" || theme == "dark" {
		policy["theme"] = theme
	}
	if size := ctx.FormValue("size"); size == "compact" || size == "flexible" {
		policy["size"] = size
	}
	if language := strings.TrimSpace(ctx.FormValue("language")); language != "" && language != "auto" {
		policy["language"] = language
	}
	if action := strings.TrimSpace(ctx.FormValue("action")); action != "" && action != "submit" {
		policy["action"] = action
	}
	if len(policy) == 0 {
		return ""
	}
	data, _ := json.Marshal(policy)
	return string(data)
}

// siteKeys returns the site keys of a profile, one for each domain. Entries
// without a key are left out.
func siteKeys(raw string) []siteKeyEntry {
	var saved, entries []siteKeyEntry
	_ = json.Unmarshal([]byte(raw), &saved)
	for _, entry := range saved {
		if strings.TrimSpace(entry.SiteKey) != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

// renderCaptchaForm shows the new or edit screen with what the owner typed.
// It never puts the secret key in the page.
func renderCaptchaForm(ctx *cartridge.Context, id uint, params integrations.CaptchaProfileParams, message string) error {
	title := "New Captcha Profile"
	if id != 0 {
		title = "Edit Captcha Profile"
	}
	keys := siteKeys(params.SiteKeysJSON)
	first := siteKeyEntry{}
	if len(keys) > 0 {
		first, keys = keys[0], keys[1:]
	}
	if first.HostPattern == "*" {
		first.HostPattern = ""
	}
	return ctx.Render("layouts/base", fiber.Map{
		"Title":       title,
		"IsEdit":      id != 0,
		"Profile":     integrations.CaptchaProfile{ID: id, Name: params.Name},
		"SiteKey":     first.SiteKey,
		"Host":        first.HostPattern,
		"MoreKeys":    keys,
		"Policy":      parseCaptchaPolicy(params.PolicyJSON, ""),
		"Error":       message,
		"ContentView": "admin/captcha/new/content",
	}, "")
}

// renderCaptchaProfile shows one profile, with the result of a test or the
// reason a delete was refused.
func renderCaptchaProfile(ctx *cartridge.Context, profile *integrations.CaptchaProfile, extra fiber.Map) error {
	data := fiber.Map{
		"Title":        profile.Name,
		"Profile":      profile,
		"SiteKeys":     siteKeys(profile.SiteKeysJSON),
		"Policy":       parseCaptchaPolicy(profile.PolicyJSON, ""),
		"Forms":        formsUsingCaptcha(ctx.DB(), profile.ID),
		"DeleteAction": "/admin/settings/captcha/" + fmt.Sprint(profile.ID) + "/delete",
		"ContentView":  "admin/captcha/show/content",
	}
	for key, value := range extra {
		data[key] = value
	}
	return ctx.Render("layouts/base", data, "")
}

// captchaProfileFromPath loads the profile that the :id of the route names.
func captchaProfileFromPath(ctx *cartridge.Context) (*integrations.CaptchaProfile, error) {
	id, err := strconv.ParseUint(ctx.Params("id"), 10, 32)
	if err != nil {
		return nil, fiber.ErrNotFound
	}
	profile, err := integrations.GetCaptchaProfileByID(ctx.DB(), uint(id))
	if err != nil {
		return nil, fiber.ErrNotFound
	}
	return profile, nil
}

// CaptchaProfileList shows all captcha profiles.
func CaptchaProfileList(ctx *cartridge.Context) error {
	profiles, err := integrations.ListCaptchaProfiles(ctx.DB())
	if err != nil {
		return fiber.ErrInternalServerError
	}

	// Add site key count to each profile
	type profileWithCount struct {
		integrations.CaptchaProfile
		SiteKeyCount int
	}
	profilesWithCount := make([]profileWithCount, len(profiles))
	for i, profile := range profiles {
		profilesWithCount[i] = profileWithCount{CaptchaProfile: profile, SiteKeyCount: len(siteKeys(profile.SiteKeysJSON))}
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Captcha Profiles",
		"Profiles":    profilesWithCount,
		"ContentView": "admin/captcha/index/content",
	}, "")
}

// CaptchaProfileNew shows the create form.
func CaptchaProfileNew(ctx *cartridge.Context) error {
	return renderCaptchaForm(ctx, 0, integrations.CaptchaProfileParams{}, "")
}

// CaptchaProfileCreate handles profile creation.
func CaptchaProfileCreate(ctx *cartridge.Context) error {
	params := integrations.CaptchaProfileParams{
		Name:         ctx.FormValue("name"),
		Provider:     ctx.FormValue("provider"),
		SecretKey:    ctx.FormValue("secret_key"),
		SiteKeysJSON: siteKeysFromForm(ctx),
		PolicyJSON:   policyFromForm(ctx, ""),
	}

	if _, err := integrations.CreateCaptchaProfile(ctx.Logger, ctx.DB(), params); err != nil {
		return renderCaptchaForm(ctx, 0, params, errorMessage(err))
	}

	return ctx.Redirect("/admin/settings/captcha")
}

// CaptchaProfileShow displays a single profile.
func CaptchaProfileShow(ctx *cartridge.Context) error {
	profile, err := captchaProfileFromPath(ctx)
	if err != nil {
		return err
	}
	return renderCaptchaProfile(ctx, profile, nil)
}

// CaptchaProfileEdit shows the edit form.
func CaptchaProfileEdit(ctx *cartridge.Context) error {
	profile, err := captchaProfileFromPath(ctx)
	if err != nil {
		return err
	}
	return renderCaptchaForm(ctx, profile.ID, integrations.CaptchaProfileParams{
		Name:         profile.Name,
		SiteKeysJSON: profile.SiteKeysJSON,
		PolicyJSON:   profile.PolicyJSON,
	}, "")
}

// CaptchaProfileUpdate handles profile updates.
func CaptchaProfileUpdate(ctx *cartridge.Context) error {
	existing, err := captchaProfileFromPath(ctx)
	if err != nil {
		return err
	}

	params := integrations.CaptchaProfileParams{
		Name:         ctx.FormValue("name"),
		Provider:     ctx.FormValue("provider"),
		SecretKey:    ctx.FormValue("secret_key"),
		SiteKeysJSON: siteKeysFromForm(ctx),
		PolicyJSON:   policyFromForm(ctx, existing.PolicyJSON),
	}
	// An empty secret means "keep the one that is saved": the form never shows it.
	if strings.TrimSpace(params.SecretKey) == "" {
		params.SecretKey = existing.SecretKey
	}

	profile, err := integrations.UpdateCaptchaProfile(ctx.Logger, ctx.DB(), existing.ID, params)
	if err != nil {
		return renderCaptchaForm(ctx, existing.ID, params, errorMessage(err))
	}

	return ctx.Redirect("/admin/settings/captcha/" + fmt.Sprint(profile.ID))
}

// CaptchaProfileDelete removes a profile that no form uses.
func CaptchaProfileDelete(ctx *cartridge.Context) error {
	profile, err := captchaProfileFromPath(ctx)
	if err != nil {
		return err
	}

	if len(formsUsingCaptcha(ctx.DB(), profile.ID)) > 0 {
		ctx.Status(fiber.StatusBadRequest)
		return renderCaptchaProfile(ctx, profile, fiber.Map{
			"Error": "A form uses this profile. Remove it from the form first.",
		})
	}

	if err := integrations.DeleteCaptchaProfile(ctx.Logger, ctx.DB(), profile.ID); err != nil {
		ctx.Logger.Error("failed to delete captcha profile", slog.Any("error", err), slog.Uint64("profile_id", uint64(profile.ID)))
		return fiber.ErrInternalServerError
	}

	return ctx.Redirect("/admin/settings/captcha")
}

// CaptchaProfileTest asks Cloudflare whether it knows the secret key of the
// profile. It cannot test the site key or the widget: only a browser can
// solve a captcha.
func CaptchaProfileTest(ctx *cartridge.Context) error {
	profile, err := captchaProfileFromPath(ctx)
	if err != nil {
		return err
	}

	result := testResult{OK: true, Message: "Cloudflare accepted the Secret Key."}
	accepted, err := middleware.CheckTurnstileSecret(profile.SecretKey)
	switch {
	case err != nil:
		result = testResult{Message: "Formlander could not ask Cloudflare: " + err.Error()}
	case !accepted:
		result = testResult{Message: "Cloudflare rejected the Secret Key. Copy it again from your Turnstile widget in the Cloudflare dashboard."}
	}
	return renderCaptchaProfile(ctx, profile, fiber.Map{"Test": result})
}
