package forms_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"formlander/internal/forms"
)

func TestSubmissionReplyAddress(t *testing.T) {
	tests := []struct {
		name     string
		dataJSON string
		want     string
	}{
		{"the email field", `{"name":"Alice","email":"alice@example.com"}`, "alice@example.com"},
		{"a field with email in its name", `{"Your Email":"alice@example.com"}`, "alice@example.com"},
		{"spaces around the address", `{"email":"  alice@example.com "}`, "alice@example.com"},
		{"the email field before other email fields", `{"billing_email":"billing@example.com","email":"alice@example.com","alt_email":"alt@example.com"}`, "alice@example.com"},
		{"the first field by name when none is named email", `{"work_email":"work@example.com","home_email":"home@example.com"}`, "home@example.com"},
		{"the next field when the first has no address", `{"email":"none","work_email":"work@example.com"}`, "work@example.com"},
		{"no email field", `{"name":"Alice","message":"write to alice@example.com"}`, ""},
		{"a field that is not text", `{"email":["alice@example.com"]}`, ""},
		{"a display name", `{"email":"Alice <alice@example.com>"}`, ""},
		{"two addresses", `{"email":"alice@example.com, bob@example.com"}`, ""},
		{"a second header after a line break", `{"email":"alice@example.com\r\nBcc: thief@example.com"}`, ""},
		{"a payload that is not JSON", `not json`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			submission := &forms.Submission{DataJSON: tt.dataJSON}

			assert.Equal(t, tt.want, submission.ReplyAddress())
		})
	}
}
