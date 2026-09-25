package http

import (
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"formlander/internal/forms"
	"formlander/internal/pkg/dbtxn"
)

// oldTemplateMarker appears only in the starter templates shipped before
// v3.1.2, in the button style every one of them carried.
const oldTemplateMarker = "linear-gradient(135deg, #2563eb, #4338ca)"

// oldTemplateLabels maps each old template's eyebrow label to its ID.
var oldTemplateLabels = map[string]string{
	`class="formlander-eyebrow">Contact<`:     "contact",
	`>Newsletter</div>`:                       "newsletter",
	`>Waitlist</div>`:                         "waitlist",
	`>Feedback</div>`:                         "feedback",
	`>Bug report</div>`:                       "bug-report",
	`class="formlander-eyebrow">Simple form<`: "blank",
}

// UpgradeOldStarterHTML replaces starter HTML saved from the old templates
// with the current version of the same template. The admin cannot edit
// this HTML, so a saved copy with the old marker is an untouched template.
// HTML without the marker is left alone, so the upgrade runs once.
func UpgradeOldStarterHTML(logger *slog.Logger, db *gorm.DB) error {
	var stale []forms.Form
	if err := db.Where("generated_html LIKE ?", "%"+oldTemplateMarker+"%").Find(&stale).Error; err != nil {
		return err
	}

	for _, form := range stale {
		template := GetTemplateByID(oldTemplateID(form.GeneratedHTML))
		if template == nil {
			continue
		}
		html := template.RenderHTML(liveFormAction(form.Slug, form.Token))
		if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
			return tx.Model(&forms.Form{}).Where("id = ?", form.ID).Update("generated_html", html).Error
		}); err != nil {
			return err
		}
		logger.Info("upgraded starter template", slog.String("form", form.Slug), slog.String("template", template.ID))
	}
	return nil
}

func oldTemplateID(html string) string {
	for label, id := range oldTemplateLabels {
		if strings.Contains(html, label) {
			return id
		}
	}
	return ""
}
