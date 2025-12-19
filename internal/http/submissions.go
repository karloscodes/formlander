package http

import (
	"encoding/json"
	"formlander/internal/forms"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"formlander/internal/pkg/cartridge"
)

type submissionWithPreview struct {
	forms.Submission
	DataJSONPreview string
}

// SubmissionList shows all submissions with pagination and filters.
func SubmissionList(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Parse pagination
	page, _ := strconv.Atoi(ctx.Query("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage := 20
	offset := (page - 1) * perPage

	// Parse filters
	formID := ctx.Query("form_id")
	rangeFilter := ctx.Query("range")

	// Build query
	query := db.Model(&forms.Submission{}).Preload("Form")

	if formID != "" {
		query = query.Where("form_id = ?", formID)
	}

	// Handle date range filter
	if rangeFilter != "" && rangeFilter != "all" {
		var startTime time.Time
		now := time.Now()
		switch rangeFilter {
		case "7d":
			startTime = now.AddDate(0, 0, -7)
		case "30d":
			startTime = now.AddDate(0, 0, -30)
		case "90d":
			startTime = now.AddDate(0, 0, -90)
		}
		if !startTime.IsZero() {
			query = query.Where("created_at >= ?", startTime)
		}
	}

	// Get total count for pagination
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return fiber.ErrInternalServerError
	}

	// Get submissions for current page
	var submissions []forms.Submission
	if err := query.Order("created_at DESC").
		Limit(perPage).
		Offset(offset).
		Find(&submissions).Error; err != nil {
		return fiber.ErrInternalServerError
	}

	// Add previews
	submissionsWithPreview := make([]submissionWithPreview, len(submissions))
	for i, sub := range submissions {
		preview := sub.DataJSON
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		// Clean up for display
		preview = strings.ReplaceAll(preview, "\n", " ")
		submissionsWithPreview[i] = submissionWithPreview{
			Submission:      sub,
			DataJSONPreview: preview,
		}
	}

	// Get all forms for filter dropdown
	var forms []forms.Form
	db.Select("id, name").Order("name ASC").Find(&forms)

	// Calculate pagination info
	totalPages := (int(totalCount) + perPage - 1) / perPage
	hasNext := page < totalPages
	hasPrev := page > 1
	nextPage := page + 1
	prevPage := page - 1

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Submissions",
		"Submissions": submissionsWithPreview,
		"Forms":       forms,
		"Page":        page,
		"NextPage":    nextPage,
		"PrevPage":    prevPage,
		"TotalPages":  totalPages,
		"TotalCount":  totalCount,
		"HasNext":     hasNext,
		"HasPrev":     hasPrev,
		"FormID":      formID,
		"Range":       rangeFilter,
		"ContentView": "admin/submissions/index/content",
	}, "")
}

// AdminSubmissionShow renders a single submission payload.
func AdminSubmissionShow(ctx *cartridge.Context) error {
	db := ctx.DB()

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
