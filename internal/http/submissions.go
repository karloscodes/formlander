package http

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"maps"
	"mime"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

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

	query, formID, rangeFilter, spam, search := filteredSubmissions(ctx)

	// Get total count for pagination
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return cartridge.NewError(500)
	}

	// Get submissions for current page
	var submissions []forms.Submission
	if err := query.Order("created_at DESC").
		Limit(perPage).
		Offset(offset).
		Find(&submissions).Error; err != nil {
		return cartridge.NewError(500)
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

	return ctx.Render("layouts/base", cartridge.Map{
		"Title":       "Submissions",
		"Submissions": submissions,
		"SpamCount":   spamCount,
		"ReturnTo":    ctx.OriginalURL(),
		"Forms":       forms,
		"Page":        page,
		"ExportURL":   strings.Replace(link(rangeFilter, spam, 1), "/admin/submissions", "/admin/submissions/export.csv", 1),
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

// filteredSubmissions returns the submissions that the filters of the
// request ask for, and the filters themselves: the form, the time, the spam
// choice ("", "no", "only"), and the search text.
func filteredSubmissions(ctx *cartridge.Context) (query *gorm.DB, formID, rangeFilter, spam, search string) {
	formID = ctx.Query("form_id")
	rangeFilter = ctx.Query("range")
	spam = ctx.Query("spam")
	search = strings.TrimSpace(ctx.Query("q"))

	query = ctx.DB().Model(&forms.Submission{}).Preload("Form")
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
	if search != "" {
		query = query.Where("data_json LIKE ?", "%"+search+"%")
	}
	days := map[string]int{"7d": 7, "30d": 30, "90d": 90}[rangeFilter]
	if days == 0 {
		rangeFilter = ""
	} else {
		query = query.Where("created_at >= ?", time.Now().AddDate(0, 0, -days))
	}
	return query, formID, rangeFilter, spam, search
}

// SubmissionsExport sends the submissions that the filters ask for as a CSV
// file: one row for each submission, one column for each field that any of
// them has.
func SubmissionsExport(ctx *cartridge.Context) error {
	query, _, _, _, _ := filteredSubmissions(ctx)
	var submissions []forms.Submission
	if err := query.Order("created_at DESC").Find(&submissions).Error; err != nil {
		return cartridge.NewError(500)
	}

	// The columns are the fields of all the rows, by name.
	rows := make([]map[string]string, len(submissions))
	seen := map[string]bool{}
	for i, submission := range submissions {
		var payload map[string]any
		_ = json.Unmarshal([]byte(submission.DataJSON), &payload)
		rows[i] = map[string]string{}
		for name, value := range payload {
			text, ok := value.(string)
			if !ok {
				encoded, _ := json.Marshal(value)
				text = string(encoded)
			}
			rows[i][name] = text
			seen[name] = true
		}
	}
	fields := slices.Sorted(maps.Keys(seen))

	var file bytes.Buffer
	out := csv.NewWriter(&file)
	// A visitor also chooses the field names, so the header row is made safe too.
	header := []string{"received", "form", "spam"}
	for _, name := range fields {
		header = append(header, csvCell(name))
	}
	_ = out.Write(header)
	for i, submission := range submissions {
		form, spam := "", "no"
		if submission.Form != nil {
			form = submission.Form.Name
		}
		if submission.IsSpam {
			spam = "yes"
		}
		record := []string{submission.CreatedAt.UTC().Format(time.RFC3339), form, spam}
		for _, name := range fields {
			record = append(record, csvCell(rows[i][name]))
		}
		_ = out.Write(record)
	}
	out.Flush()

	ctx.Set("Content-Type", "text/csv; charset=utf-8")
	ctx.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="submissions-%s.csv"`, time.Now().UTC().Format("2006-01-02")))
	return ctx.Send(file.Bytes())
}

// csvCell makes a value safe for a spreadsheet. A cell that starts with =, +,
// -, or @ is a formula there, and a visitor chose the text: a leading quote
// makes it plain text.
func csvCell(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return "'" + value
	}
	return value
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

// AdminSubmissionShow renders a single submission payload.
func AdminSubmissionShow(ctx *cartridge.Context) error {
	db := ctx.DB()

	id, err := strconv.Atoi(ctx.Params("id"))
	if err != nil {
		return cartridge.NewError(404)
	}

	var submission forms.Submission
	if err := db.Preload("Form").Preload("WebhookEvents").Preload("EmailEvents").Preload("Files").Where("id = ?", id).First(&submission).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return cartridge.NewError(404)
		}
		return cartridge.NewError(500)
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

	return ctx.Render("layouts/base", cartridge.Map{
		"Title":       "Submission",
		"Submission":  submission,
		"JSON":        prettyJSON,
		"ReplyTo":     submission.ReplyAddress(),
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
		return cartridge.NewError(404)
	}

	fileID, err := strconv.Atoi(ctx.Params("file_id"))
	if err != nil {
		return cartridge.NewError(404)
	}

	var file forms.SubmissionFile
	if err := db.Where("id = ? AND submission_id = ?", fileID, submissionID).First(&file).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return cartridge.NewError(404)
		}
		return cartridge.NewError(500)
	}

	stored, err := os.Open(forms.GetFilePath(cfg.DataDirectory, &file))
	if err != nil {
		return cartridge.NewError(404)
	}
	info, err := stored.Stat()
	if err != nil {
		stored.Close()
		return cartridge.NewError(500)
	}

	// A visitor chose the name and the type of the file. FormatMediaType keeps
	// the name inside one filename parameter, and the browser saves the bytes
	// as a download of an unknown type, never as a page of this site.
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename})
	if disposition == "" {
		disposition = "attachment"
	}
	ctx.Set("Content-Disposition", disposition)
	ctx.Set("Content-Type", "application/octet-stream")
	ctx.Set("X-Content-Type-Options", "nosniff")
	return ctx.SendStream(stored, int(info.Size()))
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
		return cartridge.NewError(404)
	}

	deleted, err := forms.DeleteSubmissions(ctx.Logger, ctx.DB(), GetAppConfig(ctx).DataDirectory, []uint{uint(id)})
	if err != nil {
		return cartridge.NewError(500)
	}
	if deleted == 0 {
		return cartridge.NewError(404)
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
		return cartridge.NewError(500)
	}

	return ctx.Redirect(returnPath(ctx, "/admin/submissions"))
}
