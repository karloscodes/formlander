package http

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"formlander/internal/integrations"
	"formlander/internal/jobs"
)

// webhookParamsFromForm reads a webhook profile from the posted form. The
// headers arrive as rows: one header_name and one header_value for each.
func webhookParamsFromForm(ctx *cartridge.Context) integrations.WebhookProfileParams {
	names := postedValues(ctx, "header_name")
	values := postedValues(ctx, "header_value")
	var headers []integrations.WebhookHeader
	for i, name := range names {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		if name != "" || value != "" {
			headers = append(headers, integrations.WebhookHeader{Name: name, Value: value})
		}
	}
	return integrations.WebhookProfileParams{
		Name:    ctx.FormValue("name"),
		URL:     ctx.FormValue("url"),
		Secret:  ctx.FormValue("secret"),
		Headers: headers,
	}
}

// renderWebhookForm shows the new or edit screen with what the owner typed.
func renderWebhookForm(ctx *cartridge.Context, id uint, params integrations.WebhookProfileParams, message string) error {
	title := "New Webhook Profile"
	if id != 0 {
		title = "Edit Webhook Profile"
	}
	return ctx.Render("layouts/base", fiber.Map{
		"Title":       title,
		"IsEdit":      id != 0,
		"Profile":     integrations.WebhookProfile{ID: id, Name: params.Name, URL: params.URL, Secret: params.Secret},
		"Headers":     params.Headers,
		"Error":       message,
		"SignedBy":    GetAppConfig(ctx).Webhook.SignatureHeader,
		"ContentView": "admin/webhooks/new/content",
	}, "")
}

// renderWebhookProfile shows one profile, with the result of a test or the
// reason a delete was refused.
func renderWebhookProfile(ctx *cartridge.Context, profile *integrations.WebhookProfile, extra fiber.Map) error {
	data := fiber.Map{
		"Title":        profile.Name,
		"Profile":      profile,
		"Headers":      profile.Headers(),
		"Forms":        formsUsingWebhook(ctx.DB(), profile.ID),
		"DeleteAction": "/admin/settings/webhooks/" + fmt.Sprint(profile.ID) + "/delete",
		"SignedBy":     GetAppConfig(ctx).Webhook.SignatureHeader,
		"ContentView":  "admin/webhooks/show/content",
	}
	for key, value := range extra {
		data[key] = value
	}
	return ctx.Render("layouts/base", data, "")
}

// webhookProfileFromPath loads the profile that the :id of the route names.
func webhookProfileFromPath(ctx *cartridge.Context) (*integrations.WebhookProfile, error) {
	id, err := strconv.ParseUint(ctx.Params("id"), 10, 32)
	if err != nil {
		return nil, fiber.ErrNotFound
	}
	profile, err := integrations.GetWebhookProfileByID(ctx.DB(), uint(id))
	if err != nil {
		return nil, fiber.ErrNotFound
	}
	return profile, nil
}

// WebhookProfileList shows all webhook profiles.
func WebhookProfileList(ctx *cartridge.Context) error {
	profiles, err := integrations.ListWebhookProfiles(ctx.DB())
	if err != nil {
		return fiber.ErrInternalServerError
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Webhook Profiles",
		"Profiles":    profiles,
		"ContentView": "admin/webhooks/index/content",
	}, "")
}

// WebhookProfileNew shows the create form.
func WebhookProfileNew(ctx *cartridge.Context) error {
	return renderWebhookForm(ctx, 0, integrations.WebhookProfileParams{}, "")
}

// WebhookProfileCreate handles profile creation.
func WebhookProfileCreate(ctx *cartridge.Context) error {
	params := webhookParamsFromForm(ctx)

	profile, err := integrations.CreateWebhookProfile(ctx.Logger, ctx.DB(), params)
	if err != nil {
		return renderWebhookForm(ctx, 0, params, errorMessage(err))
	}

	return ctx.Redirect("/admin/settings/webhooks/" + fmt.Sprint(profile.ID))
}

// WebhookProfileShow displays a single profile.
func WebhookProfileShow(ctx *cartridge.Context) error {
	profile, err := webhookProfileFromPath(ctx)
	if err != nil {
		return err
	}
	return renderWebhookProfile(ctx, profile, nil)
}

// WebhookProfileEdit shows the edit form.
func WebhookProfileEdit(ctx *cartridge.Context) error {
	profile, err := webhookProfileFromPath(ctx)
	if err != nil {
		return err
	}
	return renderWebhookForm(ctx, profile.ID, integrations.WebhookProfileParams{
		Name:    profile.Name,
		URL:     profile.URL,
		Secret:  profile.Secret,
		Headers: profile.Headers(),
	}, "")
}

// WebhookProfileUpdate handles profile updates.
func WebhookProfileUpdate(ctx *cartridge.Context) error {
	profile, err := webhookProfileFromPath(ctx)
	if err != nil {
		return err
	}
	params := webhookParamsFromForm(ctx)

	if _, err := integrations.UpdateWebhookProfile(ctx.Logger, ctx.DB(), profile.ID, params); err != nil {
		return renderWebhookForm(ctx, profile.ID, params, errorMessage(err))
	}

	return ctx.Redirect("/admin/settings/webhooks/" + fmt.Sprint(profile.ID))
}

// WebhookProfileDelete removes a profile that no form uses.
func WebhookProfileDelete(ctx *cartridge.Context) error {
	profile, err := webhookProfileFromPath(ctx)
	if err != nil {
		return err
	}

	if len(formsUsingWebhook(ctx.DB(), profile.ID)) > 0 {
		ctx.Status(fiber.StatusBadRequest)
		return renderWebhookProfile(ctx, profile, fiber.Map{
			"Error": "A form uses this profile. Remove it from the form first.",
		})
	}

	if err := integrations.DeleteWebhookProfile(ctx.Logger, ctx.DB(), profile.ID); err != nil {
		ctx.Logger.Error("failed to delete webhook profile", slog.Any("error", err), slog.Uint64("profile_id", uint64(profile.ID)))
		return fiber.ErrInternalServerError
	}

	return ctx.Redirect("/admin/settings/webhooks")
}

// WebhookProfileTest posts a sample submission to the URL of the profile and
// shows the answer.
func WebhookProfileTest(ctx *cartridge.Context) error {
	profile, err := webhookProfileFromPath(ctx)
	if err != nil {
		return err
	}

	status, err := jobs.NewWebhookDispatcher(GetAppConfig(ctx)).SendTest(ctx.UserContext(), profile)

	result := testResult{OK: true, Message: fmt.Sprintf("The webhook answered with status %d.", status)}
	switch {
	case err != nil:
		result = testResult{Message: "The request failed: " + err.Error()}
	case status < 200 || status >= 300:
		result = testResult{Message: fmt.Sprintf("The webhook answered with status %d. A delivery counts only when the status is between 200 and 299.", status)}
	}
	return renderWebhookProfile(ctx, profile, fiber.Map{"Test": result})
}
