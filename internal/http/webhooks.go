package http

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"formlander/internal/integrations"
	"formlander/internal/jobs"
)

// webhookParamsFromForm reads a webhook profile from the posted form. The
// access key has its own field and becomes the Authorization header. The
// other headers arrive as rows: one header_name and one header_value for each.
func webhookParamsFromForm(ctx *cartridge.Context) integrations.WebhookProfileParams {
	names := postedValues(ctx, "header_name")
	values := postedValues(ctx, "header_value")
	var headers []integrations.WebhookHeader
	if authorization := authorizationValue(ctx.FormValue("authorization")); authorization != "" {
		headers = append(headers, integrations.WebhookHeader{Name: "Authorization", Value: authorization})
	}
	for i, name := range names {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		// The field above is the one place for the Authorization header.
		if (name != "" || value != "") && !(len(headers) > 0 && strings.EqualFold(strings.TrimSpace(name), "Authorization")) {
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

// authorizationValue returns the value of the Authorization header for what
// the owner pasted: the whole header line, the value, or only the key. A key
// alone gets the word Bearer, which is what almost every service asks for.
func authorizationValue(pasted string) string {
	value := strings.TrimSpace(pasted)
	if rest, ok := strings.CutPrefix(strings.ToLower(value), "authorization:"); ok {
		value = strings.TrimSpace(value[len(value)-len(rest):])
	}
	if value != "" && !strings.ContainsAny(value, " \t") {
		value = "Bearer " + value
	}
	return value
}

// splitAuthorization takes the Authorization header out of the headers, for
// the field that the form has for it.
func splitAuthorization(headers []integrations.WebhookHeader) (authorization string, rest []integrations.WebhookHeader) {
	for _, header := range headers {
		if authorization == "" && strings.EqualFold(strings.TrimSpace(header.Name), "Authorization") {
			authorization = header.Value
			continue
		}
		rest = append(rest, header)
	}
	return authorization, rest
}

// renderWebhookForm shows the new or edit screen with what the owner typed.
func renderWebhookForm(ctx *cartridge.Context, id uint, params integrations.WebhookProfileParams, message string) error {
	title := "New Webhook Profile"
	if id != 0 {
		title = "Edit Webhook Profile"
	}
	authorization, headers := splitAuthorization(params.Headers)
	return ctx.Render("layouts/base", fiber.Map{
		"Title":         title,
		"IsEdit":        id != 0,
		"Profile":       integrations.WebhookProfile{ID: id, Name: params.Name, URL: params.URL, Secret: params.Secret},
		"Authorization": authorization,
		"Headers":       headers,
		"Error":         message,
		"SignedBy":      GetAppConfig(ctx).Webhook.SignatureHeader,
		"ContentView":   "admin/webhooks/new/content",
	}, "")
}

// renderWebhookProfile shows one profile, with the result of a test or the
// reason a delete was refused.
func renderWebhookProfile(ctx *cartridge.Context, profile *integrations.WebhookProfile, extra fiber.Map) error {
	authorization, headers := splitAuthorization(profile.Headers())
	data := fiber.Map{
		"Title":        profile.Name,
		"Profile":      profile,
		"HasKey":       authorization != "",
		"Headers":      headers,
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
	case status == fiber.StatusUnauthorized || status == fiber.StatusForbidden:
		result = testResult{Message: fmt.Sprintf("The webhook answered with status %d: it did not accept the access key. Edit the profile and paste the key that the service gave you into Access key.", status)}
	case status < 200 || status >= 300:
		result = testResult{Message: fmt.Sprintf("The webhook answered with status %d. A delivery counts only when the status is between 200 and 299.", status)}
	}
	return renderWebhookProfile(ctx, profile, fiber.Map{"Test": result})
}
