package http

import (
	"errors"
	nethttp "net/http"

	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"formlander/internal/forms"
)

// DemoContactForm renders a public demo contact form page.
func DemoContactForm(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Find the demo form by slug
	var form forms.Form
	if err := db.Where("slug = ?", "demo-contact").First(&form).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ctx.Status(nethttp.StatusNotFound).SendString("Demo form not found. Please create a form with slug 'demo-contact'.")
		}
		return cartridge.NewError(500)
	}

	return ctx.Render("demo", cartridge.Map{
		"FormSlug":  form.Slug,
		"FormToken": form.Token,
	}, "")
}
