package jobs

import (
	"context"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"formlander/internal/config"
	"formlander/internal/forms"
	"formlander/internal/integrations"
	"formlander/internal/pkg/testsupport"
)

// smtpProfile is a mailer profile that sends through the fake SMTP server.
func smtpProfile(host string, port int) *integrations.MailerProfile {
	return &integrations.MailerProfile{
		Name:             "SMTP relay",
		Provider:         "smtp",
		DefaultFromName:  "Forms",
		DefaultFromEmail: "forms@example.com",
		SMTPHost:         host,
		SMTPPort:         port,
		SMTPUsername:     "relay-user",
		SMTPPassword:     "relay-pass",
		SMTPEncryption:   "none",
	}
}

// toOwner is the email settings of a form that only names a recipient.
const toOwner = `{"to":"owner@example.com"}`

// dispatchSubmission stores the form "Contact" that emails through the
// profile, one submission of it, and runs the dispatcher once. It returns the
// email event after the run.
func dispatchSubmission(t *testing.T, profile *integrations.MailerProfile, overridesJSON, dataJSON string) forms.EmailEvent {
	t.Helper()
	db := testsupport.SetupTestDB(t)
	require.NoError(t, db.Create(profile).Error)
	form := &forms.Form{Name: "Contact", AllowedOrigins: "*"}
	require.NoError(t, db.Create(form).Error)
	require.NoError(t, db.Create(&forms.EmailDelivery{
		FormID:          form.ID,
		Enabled:         true,
		MailerProfileID: &profile.ID,
		OverridesJSON:   overridesJSON,
	}).Error)
	sub := &forms.Submission{FormID: form.ID, DataJSON: dataJSON}
	require.NoError(t, db.Create(sub).Error)
	event := &forms.EmailEvent{SubmissionID: sub.ID, Status: forms.WebhookStatusPending}
	require.NoError(t, db.Create(event).Error)

	require.NoError(t, NewEmailDispatcher(&config.Config{}).ProcessBatch(&JobContext{
		Context: context.Background(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      db,
	}))

	return reloadEmailEvent(t, db, event.ID)
}

// subjectOf reads the subject of a captured email the way a mail client does.
func subjectOf(t *testing.T, data string) string {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(data))
	require.NoError(t, err)
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	require.NoError(t, err)
	return subject
}

// bodiesOf reads the plain-text and HTML bodies of a captured email, by
// content type, the way a mail client does.
func bodiesOf(t *testing.T, data string) map[string]string {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(data))
	require.NoError(t, err)
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/alternative", mediaType)

	bodies := map[string]string{}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		body, err := io.ReadAll(part)
		require.NoError(t, err)
		contentType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		bodies[contentType] = strings.ReplaceAll(string(body), "\r\n", "\n")
	}
	return bodies
}

func reloadEmailEvent(t *testing.T, db *gorm.DB, id uint) forms.EmailEvent {
	t.Helper()
	var event forms.EmailEvent
	require.NoError(t, db.First(&event, id).Error)
	return event
}

func TestEmailDispatcher(t *testing.T) {
	t.Run("delivers a submission through the SMTP server of the profile", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)

		event := dispatchSubmission(t, smtpProfile(host, port), toOwner, `{"name":"Alice","email":"alice@example.com"}`)

		assert.Equal(t, forms.WebhookStatusDelivered, event.Status, "event should be marked delivered")
		captured.mu.Lock()
		defer captured.mu.Unlock()
		assert.True(t, captured.authReceived)
		assert.Contains(t, captured.from, "forms@example.com")
		assert.Contains(t, captured.to, "owner@example.com")
		assert.Equal(t, "New submission · Contact", subjectOf(t, captured.data))
		assert.Contains(t, captured.data, "Alice")
	})

	t.Run("sends the fields as plain text and as HTML", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)

		dispatchSubmission(t, smtpProfile(host, port), toOwner, `{"name":"Alice","message":"Hello\nthere","topics":["sales","support"]}`)

		captured.mu.Lock()
		defer captured.mu.Unlock()
		bodies := bodiesOf(t, captured.data)
		assert.Equal(t, "New submission to Contact\n\nmessage:\nHello\nthere\n\nname:\nAlice\n\ntopics:\nsales\nsupport\n", bodies["text/plain"])
		assert.Contains(t, bodies["text/html"], "<h1 style=\"margin:4px 0 0;font-size:20px;font-weight:600;\">Contact</h1>")
		assert.Contains(t, bodies["text/html"], ">Hello<br>there</p>")
		assert.Contains(t, bodies["text/html"], ">sales<br>support</p>")
	})

	t.Run("escapes markup that a visitor typed", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)

		dispatchSubmission(t, smtpProfile(host, port), toOwner, `{"name":"<img src=x onerror=alert(1)>"}`)

		captured.mu.Lock()
		defer captured.mu.Unlock()
		html := bodiesOf(t, captured.data)["text/html"]
		assert.Contains(t, html, "&lt;img src=x onerror=alert(1)&gt;")
		assert.NotContains(t, html, "<img")
	})

	t.Run("uses the subject that the form sets", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)

		dispatchSubmission(t, smtpProfile(host, port), `{"to":"owner@example.com","subject":"Nuevo mensaje · Tienda"}`, `{"name":"Alice"}`)

		captured.mu.Lock()
		defer captured.mu.Unlock()
		assert.Equal(t, "Nuevo mensaje · Tienda", subjectOf(t, captured.data))
	})

	t.Run("a reply goes to the person who sent the submission", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)

		dispatchSubmission(t, smtpProfile(host, port), toOwner, `{"name":"Alice","email":"alice@example.com"}`)

		captured.mu.Lock()
		defer captured.mu.Unlock()
		assert.Contains(t, captured.data, "\nReply-To: alice@example.com\n")
	})

	t.Run("sends without Reply-To when the submission has no email address", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)

		event := dispatchSubmission(t, smtpProfile(host, port), toOwner, `{"name":"Alice"}`)

		assert.Equal(t, forms.WebhookStatusDelivered, event.Status)
		captured.mu.Lock()
		defer captured.mu.Unlock()
		assert.Equal(t, "New submission · Contact", subjectOf(t, captured.data))
		assert.NotContains(t, captured.data, "Reply-To")
	})

	t.Run("keeps a header that a visitor typed into the email field out of the headers", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)

		event := dispatchSubmission(t, smtpProfile(host, port), toOwner, `{"email":"alice@example.com\r\nBcc: thief@example.com"}`)

		assert.Equal(t, forms.WebhookStatusDelivered, event.Status)
		captured.mu.Lock()
		defer captured.mu.Unlock()
		assert.Equal(t, "New submission · Contact", subjectOf(t, captured.data))
		msg, err := mail.ReadMessage(strings.NewReader(captured.data))
		require.NoError(t, err)
		assert.Empty(t, msg.Header.Get("Reply-To"))
		assert.Empty(t, msg.Header.Get("Bcc"))
	})

	t.Run("sends without Reply-To when the address is outside ASCII", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)

		event := dispatchSubmission(t, smtpProfile(host, port), toOwner, `{"email":"josé@example.com"}`)

		assert.Equal(t, forms.WebhookStatusDelivered, event.Status)
		captured.mu.Lock()
		defer captured.mu.Unlock()
		assert.Equal(t, "New submission · Contact", subjectOf(t, captured.data))
		assert.NotContains(t, captured.data, "Reply-To")
	})

	t.Run("delivers a submission through Mailgun with the Reply-To header and the subject of the form", func(t *testing.T) {
		var posted url.Values
		var path string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			posted, path = r.PostForm, r.URL.Path
		}))
		t.Cleanup(server.Close)
		previous := mailgunAPI
		mailgunAPI = server.URL
		t.Cleanup(func() { mailgunAPI = previous })
		profile := &integrations.MailerProfile{
			Name:             "Mailgun",
			Provider:         "mailgun",
			APIKey:           "key-test",
			Domain:           "mg.example.com",
			DefaultFromEmail: "forms@example.com",
		}

		event := dispatchSubmission(t, profile, `{"to":"owner@example.com","subject":"Nuevo mensaje · Tienda"}`, `{"name":"Alice","email":"alice@example.com"}`)

		assert.Equal(t, forms.WebhookStatusDelivered, event.Status)
		assert.Equal(t, "/v3/mg.example.com/messages", path)
		assert.Equal(t, "owner@example.com", posted.Get("to"))
		assert.Equal(t, "alice@example.com", posted.Get("h:Reply-To"))
		assert.Equal(t, "Nuevo mensaje · Tienda", posted.Get("subject"))
		assert.Contains(t, posted.Get("text"), "name:\nAlice")
		assert.Contains(t, posted.Get("html"), ">Alice</p>")
	})
}

func TestEmailSendTest(t *testing.T) {
	t.Run("sends a test email through the SMTP server of the profile", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)
		profile := &integrations.MailerProfile{
			Name:             "Relay",
			Provider:         "smtp",
			DefaultFromEmail: "forms@example.com",
			SMTPHost:         host,
			SMTPPort:         port,
			SMTPEncryption:   "none",
		}

		err := NewEmailDispatcher(&config.Config{}).SendTest(context.Background(), profile, "owner@example.com")

		require.NoError(t, err)
		captured.mu.Lock()
		defer captured.mu.Unlock()
		assert.Contains(t, captured.to, "owner@example.com")
		assert.Contains(t, captured.data, "Subject: Test email from Formlander")
		assert.Contains(t, captured.data, `"Relay"`)
	})

	t.Run("says what the profile lacks", func(t *testing.T) {
		tests := []struct {
			name    string
			profile integrations.MailerProfile
			want    string
		}{
			{"no From address", integrations.MailerProfile{Provider: "smtp", SMTPHost: "smtp.example.com", SMTPPort: 587}, "no From address"},
			{"no SMTP host", integrations.MailerProfile{Provider: "smtp", DefaultFromEmail: "forms@example.com"}, "no SMTP host"},
			{"no Mailgun key", integrations.MailerProfile{Provider: "mailgun", DefaultFromEmail: "forms@example.com", Domain: "mg.example.com"}, "no Mailgun API key"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := NewEmailDispatcher(&config.Config{}).SendTest(context.Background(), &tt.profile, "owner@example.com")

				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.want)
			})
		}
	})
}
