package jobs

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"formlander/internal/config"
	"formlander/internal/forms"
	"formlander/internal/integrations"
	"formlander/internal/pkg/testsupport"
)

// receivedHook is what a test receiver got from Formlander.
type receivedHook struct {
	header http.Header
	body   []byte
}

// startReceiver runs a webhook receiver that answers with status.
func startReceiver(t *testing.T, status int) (*httptest.Server, *receivedHook) {
	t.Helper()
	got := &receivedHook{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.header = r.Header.Clone()
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server, got
}

func webhookTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Webhook.SignatureHeader = "X-Formlander-Signature"
	return cfg
}

func TestWebhookSendTest(t *testing.T) {
	t.Run("posts a signed sample with the headers of the profile", func(t *testing.T) {
		server, got := startReceiver(t, http.StatusNoContent)
		profile := &integrations.WebhookProfile{
			Name:        "Receiver",
			URL:         server.URL,
			Secret:      "s3cret",
			HeadersJSON: `{"Authorization":"Bearer token"}`,
		}

		status, err := NewWebhookDispatcher(webhookTestConfig()).SendTest(context.Background(), profile)

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, status)
		assert.Equal(t, "Bearer token", got.header.Get("Authorization"))
		assert.Equal(t, "application/json", got.header.Get("Content-Type"))
		mac := hmac.New(sha256.New, []byte("s3cret"))
		mac.Write(got.body)
		assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), got.header.Get("X-Formlander-Signature"))
		var payload struct {
			Test       bool
			Submission struct{ Data map[string]string }
		}
		require.NoError(t, json.Unmarshal(got.body, &payload))
		assert.True(t, payload.Test)
		assert.Equal(t, "ada@example.com", payload.Submission.Data["email"])
	})

	t.Run("reports the status when the receiver refuses", func(t *testing.T) {
		server, _ := startReceiver(t, http.StatusUnauthorized)

		status, err := NewWebhookDispatcher(webhookTestConfig()).SendTest(context.Background(), &integrations.WebhookProfile{URL: server.URL})

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("reports an error when nothing answers", func(t *testing.T) {
		server, _ := startReceiver(t, http.StatusOK)
		server.Close()

		_, err := NewWebhookDispatcher(webhookTestConfig()).SendTest(context.Background(), &integrations.WebhookProfile{URL: server.URL})

		assert.Error(t, err)
	})
}

func TestWebhookDispatcherDeliversThroughProfile(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	server, got := startReceiver(t, http.StatusOK)
	profile := &integrations.WebhookProfile{Name: "Receiver", URL: server.URL, Secret: "s3cret"}
	require.NoError(t, db.Create(profile).Error)
	form := &forms.Form{Name: "Contact", AllowedOrigins: "*"}
	require.NoError(t, db.Create(form).Error)
	require.NoError(t, db.Create(&forms.WebhookDelivery{FormID: form.ID, Enabled: true, WebhookProfileID: &profile.ID}).Error)
	sub := &forms.Submission{FormID: form.ID, DataJSON: `{"name":"Alice"}`}
	require.NoError(t, db.Create(sub).Error)
	event := &forms.WebhookEvent{SubmissionID: sub.ID, Status: forms.WebhookStatusPending}
	require.NoError(t, db.Create(event).Error)
	ctx := &JobContext{
		Context: context.Background(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      db,
	}

	require.NoError(t, NewWebhookDispatcher(webhookTestConfig()).ProcessBatch(ctx))

	var updated forms.WebhookEvent
	require.NoError(t, db.First(&updated, event.ID).Error)
	assert.Equal(t, forms.WebhookStatusDelivered, updated.Status, updated.LastAttemptErr)
	assert.Contains(t, string(got.body), `"name":"Alice"`)
	assert.NotEmpty(t, got.header.Get("X-Formlander-Signature"))
}
