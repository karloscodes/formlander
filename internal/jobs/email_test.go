package jobs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"formlander/internal/config"
	"formlander/internal/integrations"
)

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
