package http

import (
	"errors"
	"formlander/internal/forms"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"formlander/internal/pkg/cartridge"
)

// DemoContactForm renders a public demo contact form page.
func DemoContactForm(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

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
