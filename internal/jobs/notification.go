package jobs

import (
	"encoding/json"
	"html/template"
	"maps"
	"slices"
	"strings"
)

// notificationField is one field of a submission, as the notification email
// shows it.
type notificationField struct {
	Name  string
	Lines []string
}

// submissionFields returns the fields of a submission, sorted by name. The
// form does not keep the order of its fields, so the name order makes every
// email show them in the same order.
func submissionFields(dataJSON string) []notificationField {
	var payload map[string]any
	if err := json.Unmarshal([]byte(dataJSON), &payload); err != nil {
		payload = map[string]any{"raw": dataJSON}
	}

	fields := make([]notificationField, 0, len(payload))
	for _, name := range slices.Sorted(maps.Keys(payload)) {
		lines := strings.Split(crlf(plainValue(payload[name])), "\r\n")
		fields = append(fields, notificationField{Name: name, Lines: lines})
	}
	return fields
}

// plainValue returns a field value as text: a string as the visitor typed it,
// a list with one item on each line, and any other value as JSON.
func plainValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = plainValue(item)
		}
		return strings.Join(items, "\n")
	default:
		encoded, _ := json.Marshal(v)
		return string(encoded)
	}
}

// notificationText returns the plain-text body of the notification email.
func notificationText(formName string, fields []notificationField) string {
	var b strings.Builder
	b.WriteString("New submission to " + formName + "\n")
	for _, field := range fields {
		b.WriteString("\n" + field.Name + ":\n" + strings.Join(field.Lines, "\n") + "\n")
	}
	return b.String()
}

// notificationHTML returns the HTML body of the notification email, or "" when
// the template fails, so that the email goes out as plain text only. The
// template escapes every value, so a visitor cannot add markup to the email.
func notificationHTML(formName string, fields []notificationField) string {
	var b strings.Builder
	data := map[string]any{"Form": formName, "Fields": fields}
	if err := notificationTemplate.Execute(&b, data); err != nil {
		return ""
	}
	return b.String()
}

// notificationTemplate uses tables and inline styles, because many email
// clients ignore a style sheet.
var notificationTemplate = template.Must(template.New("notification").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body style="margin:0;padding:24px 12px;background:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;color:#18181b;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:600px;margin:0 auto;background:#ffffff;border:1px solid #e4e4e7;border-radius:8px;">
<tr><td style="padding:24px 24px 16px;">
<p style="margin:0;font-size:13px;color:#71717a;">New submission</p>
<h1 style="margin:4px 0 0;font-size:20px;font-weight:600;">{{.Form}}</h1>
</td></tr>
{{- range .Fields}}
<tr><td style="padding:12px 24px;border-top:1px solid #e4e4e7;">
<p style="margin:0 0 4px;font-size:13px;color:#71717a;">{{.Name}}</p>
<p style="margin:0;font-size:15px;line-height:1.5;word-break:break-word;">{{range $i, $line := .Lines}}{{if $i}}<br>{{end}}{{$line}}{{end}}</p>
</td></tr>
{{- end}}
</table>
<p style="max-width:600px;margin:16px auto 0;font-size:12px;color:#a1a1aa;text-align:center;">Sent by Formlander</p>
</body>
</html>
`))
