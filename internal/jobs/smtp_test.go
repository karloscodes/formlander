package jobs

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"formlander/internal/config"
	"formlander/internal/forms"
	"formlander/internal/integrations"
	"formlander/internal/pkg/testsupport"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedMail records what a fake SMTP server received.
type capturedMail struct {
	mu           sync.Mutex
	from         string
	to           string
	data         string
	authReceived bool
}

// startFakeSMTPServer spins up a minimal plaintext SMTP server on a random
// loopback port for one connection, capturing the envelope and message.
func startFakeSMTPServer(t *testing.T) (host string, port int, captured *capturedMail) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	captured = &capturedMail{}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

		r := bufio.NewReader(conn)
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }

		write("220 fake ESMTP")
		inData := false
		var dataLines []string
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")

			if inData {
				if line == "." {
					captured.mu.Lock()
					captured.data = strings.Join(dataLines, "\n")
					captured.mu.Unlock()
					inData = false
					write("250 OK queued")
					continue
				}
				dataLines = append(dataLines, line)
				continue
			}

			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				write("250-fake greets you")
				write("250 AUTH PLAIN LOGIN")
			case strings.HasPrefix(line, "AUTH"):
				captured.mu.Lock()
				captured.authReceived = true
				captured.mu.Unlock()
				write("235 2.7.0 Authentication successful")
			case strings.HasPrefix(line, "MAIL FROM:"):
				captured.mu.Lock()
				captured.from = line
				captured.mu.Unlock()
				write("250 OK")
			case strings.HasPrefix(line, "RCPT TO:"):
				captured.mu.Lock()
				captured.to = line
				captured.mu.Unlock()
				write("250 OK")
			case strings.HasPrefix(line, "DATA"):
				write("354 End data with <CR><LF>.<CR><LF>")
				inData = true
			case strings.HasPrefix(line, "QUIT"):
				write("221 Bye")
				return
			default:
				write("250 OK")
			}
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port, captured
}

func TestSendSMTP(t *testing.T) {
	t.Run("delivers with PLAIN auth over plaintext", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)
		cfg := &smtpConfig{
			Host: host, Port: port,
			Username: "apikey", Password: "secret",
			Encryption: "none",
			From:       "Forms <forms@example.com>",
			To:         "owner@example.com",
		}
		msg := buildSMTPMessage(cfg.From, cfg.To, "New submission", "name: Alice")

		err := sendSMTP(cfg, msg)
		require.NoError(t, err)

		captured.mu.Lock()
		defer captured.mu.Unlock()
		assert.True(t, captured.authReceived, "expected AUTH command")
		assert.Contains(t, captured.from, "forms@example.com")
		assert.Contains(t, captured.to, "owner@example.com")
		assert.Contains(t, captured.data, "Subject: New submission")
		assert.Contains(t, captured.data, "name: Alice")
	})

	t.Run("skips auth when username empty", func(t *testing.T) {
		host, port, captured := startFakeSMTPServer(t)
		cfg := &smtpConfig{Host: host, Port: port, Encryption: "none", From: "a@x.com", To: "b@x.com"}

		err := sendSMTP(cfg, buildSMTPMessage(cfg.From, cfg.To, "S", "B"))
		require.NoError(t, err)

		captured.mu.Lock()
		defer captured.mu.Unlock()
		assert.False(t, captured.authReceived, "expected no AUTH when username empty")
		assert.Contains(t, captured.from, "a@x.com")
	})
}

func TestEmailDispatcherDeliversViaSMTP(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	host, port, captured := startFakeSMTPServer(t)

	profile := &integrations.MailerProfile{
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
	require.NoError(t, db.Create(profile).Error)

	form := &forms.Form{Name: "Contact", AllowedOrigins: "*"}
	require.NoError(t, db.Create(form).Error)

	pid := profile.ID
	require.NoError(t, db.Create(&forms.EmailDelivery{
		FormID:          form.ID,
		Enabled:         true,
		MailerProfileID: &pid,
		OverridesJSON:   `{"to":"owner@example.com"}`,
	}).Error)

	sub := &forms.Submission{FormID: form.ID, DataJSON: `{"name":"Alice","email":"alice@example.com"}`}
	require.NoError(t, db.Create(sub).Error)

	event := &forms.EmailEvent{SubmissionID: sub.ID, Status: forms.WebhookStatusPending}
	require.NoError(t, db.Create(event).Error)

	d := NewEmailDispatcher(&config.Config{})
	ctx := &JobContext{
		Context: context.Background(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      db,
	}

	require.NoError(t, d.ProcessBatch(ctx))

	var updated forms.EmailEvent
	require.NoError(t, db.First(&updated, event.ID).Error)
	assert.Equal(t, forms.WebhookStatusDelivered, updated.Status, "event should be marked delivered")

	captured.mu.Lock()
	defer captured.mu.Unlock()
	assert.True(t, captured.authReceived)
	assert.Contains(t, captured.from, "forms@example.com")
	assert.Contains(t, captured.to, "owner@example.com")
	assert.Contains(t, captured.data, "Subject: New submission")
	assert.Contains(t, captured.data, "Alice")
}

func TestBuildSMTPMessage(t *testing.T) {
	t.Run("includes RFC 5322 headers and body", func(t *testing.T) {
		msg := buildSMTPMessage("Forms <forms@example.com>", "owner@example.com", "New submission", "name: Alice\nemail: alice@x.com")

		s := string(msg)
		if !strings.Contains(s, "From: Forms <forms@example.com>\r\n") {
			t.Errorf("missing From header, got:\n%s", s)
		}
		if !strings.Contains(s, "To: owner@example.com\r\n") {
			t.Errorf("missing To header, got:\n%s", s)
		}
		if !strings.Contains(s, "Subject: New submission\r\n") {
			t.Errorf("missing Subject header, got:\n%s", s)
		}
		if !strings.Contains(s, "MIME-Version: 1.0\r\n") {
			t.Errorf("missing MIME-Version header, got:\n%s", s)
		}
		if !strings.Contains(s, "Content-Type: text/plain; charset=\"utf-8\"\r\n") {
			t.Errorf("missing Content-Type header, got:\n%s", s)
		}
	})

	t.Run("separates headers from body with a blank CRLF line", func(t *testing.T) {
		msg := buildSMTPMessage("a@x.com", "b@x.com", "Hi", "line one\nline two")

		s := string(msg)
		if !strings.Contains(s, "\r\n\r\n") {
			t.Errorf("expected blank line separating headers and body, got:\n%s", s)
		}
		// body newlines normalized to CRLF
		if !strings.Contains(s, "line one\r\nline two") {
			t.Errorf("expected body lines joined with CRLF, got:\n%s", s)
		}
	})
}
