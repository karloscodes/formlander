package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"
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
			return ctx.Status(fiber.StatusNotFound).SendString("Demo form not found. Please create a form with slug 'demo-contact'.")
		}
		return fiber.ErrInternalServerError
	}

	return ctx.Render("demo", fiber.Map{
		"FormSlug":  form.Slug,
		"FormToken": form.Token,
	}, "")
}
