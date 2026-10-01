package http

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"
	"gorm.io/gorm"

	"formlander/internal/forms"
)

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
	search := strings.TrimSpace(ctx.Query("q"))
	spam := ctx.Query("spam")
	if rangeFilter == "all" {
		rangeFilter = ""
	}

	// Build query
	query := db.Model(&forms.Submission{}).Preload("Form")

	if formID != "" {
		query = query.Where("form_id = ?", formID)
	}

	switch spam {
	case "only":
		query = query.Where("is_spam = ?", true)
	case "no":
		query = query.Where("is_spam = ?", false)
	default:
		spam = ""
	}

	// Search in data_json
	if search != "" {
		query = query.Where("data_json LIKE ?", "%"+search+"%")
	}

	// Handle date range filter
	if rangeFilter != "" {
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

	// Count the spam that "Delete spam" removes: all of it, or that of the
	// chosen form.
	spamQuery := db.Model(&forms.Submission{}).Where("is_spam = ?", true)
	if formID != "" {
		spamQuery = spamQuery.Where("form_id = ?", formID)
	}
	var spamCount int64
	spamQuery.Count(&spamCount)

	// Get all forms for filter dropdown
	var forms []forms.Form
	db.Select("id, name").Order("name ASC").Find(&forms)

	// Calculate pagination info
	totalPages := (int(totalCount) + perPage - 1) / perPage
	hasNext := page < totalPages
	hasPrev := page > 1
	nextPage := page + 1
	prevPage := page - 1

	// Each choice of a filter is a link that keeps the other filters.
	link := func(rangeFilter, spam string, page int) string {
		return submissionsURL(formID, rangeFilter, spam, search, page)
	}
	ranges := []filterLink{
		{"All time", link("", spam, 1), rangeFilter == ""},
		{"7 days", link("7d", spam, 1), rangeFilter == "7d"},
		{"30 days", link("30d", spam, 1), rangeFilter == "30d"},
		{"90 days", link("90d", spam, 1), rangeFilter == "90d"},
	}
	spamChoices := []filterLink{
		{"All", link(rangeFilter, "", 1), spam == ""},
		{"No spam", link(rangeFilter, "no", 1), spam == "no"},
		{"Spam", link(rangeFilter, "only", 1), spam == "only"},
	}

	return ctx.Render("layouts/base", fiber.Map{
		"Title":       "Submissions",
		"Submissions": submissions,
		"SpamCount":   spamCount,
		"ReturnTo":    ctx.OriginalURL(),
		"Forms":       forms,
		"Page":        page,
		"NextURL":     link(rangeFilter, spam, nextPage),
		"PrevURL":     link(rangeFilter, spam, prevPage),
		"TotalPages":  totalPages,
		"TotalCount":  totalCount,
		"HasNext":     hasNext,
		"HasPrev":     hasPrev,
		"FormID":      formID,
		"Range":       rangeFilter,
		"Spam":        spam,
		"Search":      search,
		"Filtered":    formID != "" || rangeFilter != "" || spam != "" || search != "",
		"Ranges":      ranges,
		"SpamChoices": spamChoices,
		"ContentView": "admin/submissions/index/content",
	}, "")
}

// filterLink is one choice of a filter of the submissions list.
type filterLink struct {
	Label string
	URL   string
	On    bool
}

// submissionsURL returns the address of the submissions list with these
// filters. An empty filter and the first page are left out.
func submissionsURL(formID, rangeFilter, spam, search string, page int) string {
	query := url.Values{}
	for name, value := range map[string]string{"form_id": formID, "range": rangeFilter, "spam": spam, "q": search} {
		if value != "" {
			query.Set(name, value)
		}
	}
	if page > 1 {
		query.Set("page", strconv.Itoa(page))
	}
	if len(query) == 0 {
		return "/admin/submissions"
	}
	return "/admin/submissions?" + query.Encode()
}

// replyAddress returns the email address of the person who sent a
// submission, or "" when no field has one.
func replyAddress(dataJSON string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(dataJSON), &payload) != nil {
		return ""
	}
	for name, value := range payload {
		text, _ := value.(string)
		if text = strings.TrimSpace(text); strings.Contains(strings.ToLower(name), "email") && strings.Contains(text, "@") && !strings.ContainsAny(text, " \n\r") {
			return text
		}
	}
	return ""
}

// AdminSubmissionShow renders a single submission payload.
func AdminSubmissionShow(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return fiber.ErrNotFound
	}

	var submission forms.Submission
	if err := db.Preload("Form").Preload("WebhookEvents").Preload("EmailEvents").Preload("Files").Where("id = ?", id).First(&submission).Error; err != nil {
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
		"ReplyTo":     replyAddress(submission.DataJSON),
		"ReturnTo":    cameFrom(ctx, fmt.Sprintf("/admin/forms/%d", submission.FormID)),
		"ContentView": "admin/submissions/show/content",
	}, "")
}

// AdminSubmissionFileDownload serves a file from a submission.
func AdminSubmissionFileDownload(ctx *cartridge.Context) error {
	db := ctx.DB()
	cfg := GetAppConfig(ctx)

	submissionID, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return fiber.ErrNotFound
	}

	fileID, err := strconv.Atoi(ctx.Params("file_id"))
	if err != nil {
		return fiber.ErrNotFound
	}

	var file forms.SubmissionFile
	if err := db.Where("id = ? AND submission_id = ?", fileID, submissionID).First(&file).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fiber.ErrNotFound
		}
		return fiber.ErrInternalServerError
	}

	filePath := forms.GetFilePath(cfg.DataDirectory, &file)

	// Set content disposition for download
	ctx.Set("Content-Disposition", "attachment; filename=\""+file.Filename+"\"")
	if file.ContentType != "" {
		ctx.Set("Content-Type", file.ContentType)
	}

	return ctx.SendFile(filePath)
}

// cameFrom returns the admin page that linked to this one, or fallback. After
// a delete, the owner goes back there.
func cameFrom(ctx *cartridge.Context, fallback string) string {
	referer, err := url.Parse(ctx.Get("Referer"))
	if err != nil || referer.Host != ctx.Hostname() {
		return fallback
	}
	if !isAdminPath(referer.Path) || referer.Path == ctx.Path() {
		return fallback
	}
	return referer.RequestURI()
}

// isAdminPath reports whether a path is a page of the admin.
func isAdminPath(path string) bool {
	return path == "/admin" || strings.HasPrefix(path, "/admin/")
}

// returnPath is where the owner goes after a delete: the admin page that the
// form names, or fallback. Only paths inside the admin are followed, so the
// field cannot send a person to another site.
func returnPath(ctx *cartridge.Context, fallback string) string {
	path := ctx.FormValue("return_to")
	if target, err := url.Parse(path); err == nil && target.Host == "" && target.Scheme == "" && isAdminPath(target.Path) {
		return path
	}
	return fallback
}

// AdminSubmissionDelete removes one submission and its files.
func AdminSubmissionDelete(ctx *cartridge.Context) error {
	id, err := strconv.ParseUint(ctx.Params("id"), 10, 32)
	if err != nil {
		return fiber.ErrNotFound
	}

	deleted, err := forms.DeleteSubmissions(ctx.Logger, ctx.DB(), GetAppConfig(ctx).DataDirectory, []uint{uint(id)})
	if err != nil {
		return fiber.ErrInternalServerError
	}
	if deleted == 0 {
		return fiber.ErrNotFound
	}

	return ctx.Redirect(returnPath(ctx, "/admin/submissions"))
}

// AdminSubmissionsDelete removes the submissions that the owner ticked in a
// list (the "ids" fields), or all spam when the "spam" button sent the form.
func AdminSubmissionsDelete(ctx *cartridge.Context) error {
	db := ctx.DB()
	dataDir := GetAppConfig(ctx).DataDirectory

	var err error
	if ctx.FormValue("spam") != "" {
		formID, _ := strconv.ParseUint(ctx.FormValue("form_id"), 10, 32)
		_, err = forms.DeleteSpam(ctx.Logger, db, dataDir, uint(formID))
	} else {
		var ids []uint
		for _, value := range postedValues(ctx, "ids") {
			if id, parseErr := strconv.ParseUint(value, 10, 32); parseErr == nil {
				ids = append(ids, uint(id))
			}
		}
		_, err = forms.DeleteSubmissions(ctx.Logger, db, dataDir, ids)
	}
	if err != nil {
		return fiber.ErrInternalServerError
	}

	return ctx.Redirect(returnPath(ctx, "/admin/submissions"))
}
