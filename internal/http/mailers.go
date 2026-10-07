package http

import (
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"formlander/internal/accounts"
	"formlander/internal/integrations"
	"formlander/internal/jobs"
)

// parseSMTPPort converts the submitted port to an int, defaulting to 587
// (submission/STARTTLS) when the field is left blank.
func parseSMTPPort(s string) int {
	if s == "" {
		return 587
	}
	n, _ := strconv.Atoi(s)
	return n
}

// sameSMTPServer reports whether the posted profile sends to the host that
// the saved SMTP password was typed for. A saved password goes to no other
// host: the screen never shows it, so a changed host must not reveal it. A
// new port or encryption on the same host is fine.
func sameSMTPServer(params integrations.MailerProfileParams, saved *integrations.MailerProfile) bool {
	return strings.TrimSpace(params.Provider) == saved.Provider &&
		strings.EqualFold(strings.TrimSpace(params.SMTPHost), saved.SMTPHost)
}

// mailerParamsFromForm reads a mailer profile from the posted form. existing
// is the saved profile on an update, or nil on a create. The screen never
// shows a saved password or API key, so an empty one means "keep it".
func mailerParamsFromForm(ctx *cartridge.Context, existing *integrations.MailerProfile) integrations.MailerProfileParams {
	params := integrations.MailerProfileParams{
		Name:             ctx.FormValue("name"),
		Provider:         ctx.FormValue("provider"),
		APIKey:           ctx.FormValue("api_key"),
		Domain:           ctx.FormValue("domain"),
		DefaultFromName:  ctx.FormValue("default_from_name"),
		DefaultFromEmail: ctx.FormValue("default_from_email"),
		DefaultsJSON:     ctx.FormValue("defaults_json"),
		SMTPHost:         ctx.FormValue("smtp_host"),
		SMTPPort:         parseSMTPPort(ctx.FormValue("smtp_port")),
		SMTPUsername:     ctx.FormValue("smtp_username"),
		SMTPPassword:     ctx.FormValue("smtp_password"),
		SMTPEncryption:   ctx.FormValue("smtp_encryption"),
	}
	if existing != nil {
		if strings.TrimSpace(params.APIKey) == "" {
			params.APIKey = existing.APIKey
		}
		if strings.TrimSpace(params.SMTPPassword) == "" && sameSMTPServer(params, existing) {
			params.SMTPPassword = existing.SMTPPassword
		}
		// The screen has no field for the extra defaults. Keep what is saved.
		if strings.TrimSpace(params.DefaultsJSON) == "" {
			params.DefaultsJSON = existing.DefaultsJSON
		}
	}
	return params
}

// renderMailerForm shows the new or edit screen with what the owner typed.
// It leaves the password and the API key out of the page.
func renderMailerForm(ctx *cartridge.Context, id uint, params integrations.MailerProfileParams, message string) error {
	title := "New Mailer Profile"
	if id != 0 {
		title = "Edit Mailer Profile"
	}
	if params.Provider == "" {
		params.Provider = "smtp"
	}
	return ctx.Render("layouts/base", fiber.Map{
		"Title":  title,
		"IsEdit": id != 0,
		"Profile": integrations.MailerProfile{
			ID:               id,
			Name:             params.Name,
			Provider:         params.Provider,
			Domain:           params.Domain,
			DefaultFromName:  params.DefaultFromName,
			DefaultFromEmail: params.DefaultFromEmail,
			SMTPHost:         params.SMTPHost,
			SMTPPort:         params.SMTPPort,
			SMTPUsername:     params.SMTPUsername,
			SMTPEncryption:   params.SMTPEncryption,
		},
		"Error":       message,
		"ContentView": "admin/mailers/new/content",
	}, "")
}

// renderMailerProfile shows one profile, with the result of a test or the
// reason a delete was refused.
func renderMailerProfile(ctx *cartridge.Context, profile *integrations.MailerProfile, extra fiber.Map) error {
	data := fiber.Map{
		"Title":        profile.Name,
		"Profile":      profile,
		"Forms":        formsUsingMailer(ctx.DB(), profile.ID),
		"DeleteAction": "/admin/settings/mailers/" + fmt.Sprint(profile.ID) + "/delete",
		"TestTo":       adminEmail(ctx),
		"ContentView":  "admin/mailers/show/content",
	}
	for key, value := range extra {
		data[key] = value
	}
	return ctx.Render("layouts/base", data, "")
}

// adminEmail returns the address of the person who is signed in, or "".
func adminEmail(ctx *cartridge.Context) string {
	userID, ok := GetSession(ctx).GetUserID(ctx.Ctx)
	if !ok {
		return ""
	}
	user, err := accounts.FindByID(ctx.DB(), userID)
	if err != nil {
		return ""
	}
	return user.Email
}

// mailerProfileFromPath loads the profile that the :id of the route names.
func mailerProfileFromPath(ctx *cartridge.Context) (*integrations.MailerProfile, error) {
	id, err := strconv.ParseUint(ctx.Params("id"), 10, 32)
	if err != nil {
		return nil, fiber.ErrNotFound
	}
	profile, err := integrations.GetMailerProfileByID(ctx.DB(), uint(id))
	if err != nil {
		return nil, fiber.ErrNotFound
	}
	return profile, nil
}

// MailerProfileList shows all mailer profiles.
func MailerProfileList(ctx *cartridge.Context) error {
	profiles, err := integrations.ListMailerProfiles(ctx.DB())
	if err != nil {
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
	return renderMailerForm(ctx, 0, integrations.MailerProfileParams{SMTPPort: 587}, "")
}

// MailerProfileCreate handles profile creation.
func MailerProfileCreate(ctx *cartridge.Context) error {
	params := mailerParamsFromForm(ctx, nil)

	if _, err := integrations.CreateMailerProfile(ctx.Logger, ctx.DB(), params); err != nil {
		return renderMailerForm(ctx, 0, params, errorMessage(err))
	}

	return ctx.Redirect("/admin/settings/mailers")
}

// MailerProfileShow displays a single profile.
func MailerProfileShow(ctx *cartridge.Context) error {
	profile, err := mailerProfileFromPath(ctx)
	if err != nil {
		return err
	}
	return renderMailerProfile(ctx, profile, nil)
}

// MailerProfileEdit shows the edit form.
func MailerProfileEdit(ctx *cartridge.Context) error {
	profile, err := mailerProfileFromPath(ctx)
	if err != nil {
		return err
	}
	return renderMailerForm(ctx, profile.ID, integrations.MailerProfileParams{
		Name:             profile.Name,
		Provider:         profile.Provider,
		Domain:           profile.Domain,
		DefaultFromName:  profile.DefaultFromName,
		DefaultFromEmail: profile.DefaultFromEmail,
		SMTPHost:         profile.SMTPHost,
		SMTPPort:         profile.SMTPPort,
		SMTPUsername:     profile.SMTPUsername,
		SMTPEncryption:   profile.SMTPEncryption,
	}, "")
}

// MailerProfileUpdate handles profile updates.
func MailerProfileUpdate(ctx *cartridge.Context) error {
	existing, err := mailerProfileFromPath(ctx)
	if err != nil {
		return err
	}
	params := mailerParamsFromForm(ctx, existing)
	if params.Provider != "mailgun" && existing.SMTPPassword != "" && strings.TrimSpace(params.SMTPPassword) == "" {
		ctx.Status(fiber.StatusBadRequest)
		return renderMailerForm(ctx, existing.ID, params, "Type the SMTP password again. Formlander sends a saved password only to the server it was saved for.")
	}

	profile, err := integrations.UpdateMailerProfile(ctx.Logger, ctx.DB(), existing.ID, params)
	if err != nil {
		return renderMailerForm(ctx, existing.ID, params, errorMessage(err))
	}

	return ctx.Redirect("/admin/settings/mailers/" + fmt.Sprint(profile.ID))
}

// MailerProfileDelete removes a profile that no form uses.
func MailerProfileDelete(ctx *cartridge.Context) error {
	profile, err := mailerProfileFromPath(ctx)
	if err != nil {
		return err
	}

	if len(formsUsingMailer(ctx.DB(), profile.ID)) > 0 {
		ctx.Status(fiber.StatusBadRequest)
		return renderMailerProfile(ctx, profile, fiber.Map{
			"Error": "A form uses this profile. Remove it from the form first.",
		})
	}

	if err := integrations.DeleteMailerProfile(ctx.Logger, ctx.DB(), profile.ID); err != nil {
		ctx.Logger.Error("failed to delete mailer profile", slog.Any("error", err), slog.Uint64("profile_id", uint64(profile.ID)))
		return fiber.ErrInternalServerError
	}

	return ctx.Redirect("/admin/settings/mailers")
}

// MailerProfileTest sends a test email through the profile, to the address
// the owner typed, and shows whether the provider took it.
func MailerProfileTest(ctx *cartridge.Context) error {
	profile, err := mailerProfileFromPath(ctx)
	if err != nil {
		return err
	}

	to := strings.TrimSpace(ctx.FormValue("to"))
	address, err := mail.ParseAddress(to)
	if err != nil {
		return renderMailerProfile(ctx, profile, fiber.Map{
			"Test":   testResult{Message: "Type the email address that gets the test."},
			"TestTo": to,
		})
	}

	result := testResult{OK: true, Message: "The provider accepted a test email to " + address.Address + ". Look in that inbox, and in its spam folder."}
	if err := jobs.NewEmailDispatcher(GetAppConfig(ctx)).SendTest(ctx.UserContext(), profile, address.Address); err != nil {
		result = testResult{Message: "The email was not sent: " + err.Error()}
	}
	return renderMailerProfile(ctx, profile, fiber.Map{"Test": result, "TestTo": address.Address})
}
