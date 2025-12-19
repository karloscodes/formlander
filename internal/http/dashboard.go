package http

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"formlander/internal/forms"
)

// AdminDashboard shows the main dashboard with stats and recent activity
func AdminDashboard(ctx *cartridge.Context) error {
	db := ctx.DB()

	// Get total counts
	var totalForms, totalSubmissions int64
	db.Model(&forms.Form{}).Count(&totalForms)
	db.Model(&forms.Submission{}).Count(&totalSubmissions)

	// Get submissions from last 24 hours
	yesterday := time.Now().Add(-24 * time.Hour)
	var submissionsLast24h int64
	db.Model(&forms.Submission{}).Where("created_at > ?", yesterday).Count(&submissionsLast24h)

	// Get recent submissions (last 10)
	var recentSubmissions []forms.Submission
	db.Preload("Form").
		Order("created_at DESC").
		Limit(10).
		Find(&recentSubmissions)

	// Get recent forms (last 5)
	var recentForms []forms.Form
	db.Order("created_at DESC").
		Limit(5).
		Find(&recentForms)

	// Get forms with submission counts
	type FormWithCount struct {
		forms.Form
		SubmissionCount int64
	}
	var formsWithCounts []FormWithCount
	db.Model(&forms.Form{}).
		Select("forms.*, COUNT(submissions.id) as submission_count").
		Joins("LEFT JOIN submissions ON submissions.form_id = forms.id").
		Group("forms.id").
		Order("submission_count DESC").
		Limit(5).
		Find(&formsWithCounts)

	return ctx.Render("layouts/base", fiber.Map{
		"Title": "Dashboard",
		"Stats": fiber.Map{
			"TotalForms":         totalForms,
			"TotalSubmissions":   totalSubmissions,
			"SubmissionsLast24h": submissionsLast24h,
		},
		"RecentSubmissions": recentSubmissions,
		"RecentForms":       recentForms,
		"TopForms":          formsWithCounts,
		"ContentView":       "admin/dashboard/content",
	}, "")
}
