package http

import (
	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"formlander/internal/forms"
	"formlander/internal/integrations"
)

// Connections are the outside services that forms use: mailers, captchas,
// and webhooks. The owner sets each one up once, as a profile, and many
// forms can use it.

// testResult is what a "Send a test" action found. The page of the profile
// shows it as a notice.
type testResult struct {
	OK      bool
	Message string
}

// postedValues returns every value of a field that a form sends several
// times, in the order of the page.
func postedValues(ctx *cartridge.Context, name string) []string {
	ctx.Body() // keeps the body readable for later reads
	if err := ctx.Request().ParseForm(); err != nil {
		return nil
	}
	return ctx.Request().PostForm[name]
}

// errorMessage is what the owner reads when a profile cannot be saved.
func errorMessage(err error) string {
	if valErr, ok := err.(*integrations.ValidationError); ok {
		return valErr.Message
	}
	return err.Error()
}

// formsWithIDs returns the forms that a query of form IDs selects, by name.
func formsWithIDs(db *gorm.DB, formIDs *gorm.DB) []forms.Form {
	var list []forms.Form
	db.Select("id", "name").Where("id IN (?)", formIDs).Order("name ASC").Find(&list)
	return list
}

// formsUsingMailer returns the forms that send email through a mailer profile.
func formsUsingMailer(db *gorm.DB, profileID uint) []forms.Form {
	return formsWithIDs(db, db.Model(&forms.EmailDelivery{}).Select("form_id").Where("mailer_profile_id = ?", profileID))
}

// formsUsingWebhook returns the forms that post to a webhook profile.
func formsUsingWebhook(db *gorm.DB, profileID uint) []forms.Form {
	return formsWithIDs(db, db.Model(&forms.WebhookDelivery{}).Select("form_id").Where("webhook_profile_id = ?", profileID))
}

// formsUsingCaptcha returns the forms that a captcha profile protects.
func formsUsingCaptcha(db *gorm.DB, profileID uint) []forms.Form {
	var list []forms.Form
	db.Select("id", "name").Where("captcha_profile_id = ?", profileID).Order("name ASC").Find(&list)
	return list
}
