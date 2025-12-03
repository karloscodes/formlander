package controllers

import (
	"encoding/json"
	"formlander/internal/forms"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"formlander/internal/pkg/cartridge"
)

// AdminSubmissionShow renders a single submission payload.
func AdminSubmissionShow(ctx *cartridge.Context) error {
	db, err := ctx.DB()
	if err != nil {
		return fiber.ErrInternalServerError
	}

	id, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return fiber.ErrNotFound
	}

	var submission forms.Submission
	if err := db.Preload("Form").Preload("WebhookEvents").Preload("EmailEvents").Where("id = ?", id).First(&submission).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}

	var prettyJSON string
	if submission.DataJSON != "" {
		var buf any
		if err := json.Unmarshal([]byte(submission.DataJSON), &buf); err == nil {
			formatted, _ := json.MarshalIndent(buf, "", "  ")
			prettyJSON = string(formatted)
		} else {
			prettyJSON = submission.DataJSON
		}
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Submission",
		"Submission":  submission,
		"JSON":        prettyJSON,
		"ContentView": "admin/submissions/show/content",
	}, "")
}
