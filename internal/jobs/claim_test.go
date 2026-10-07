package jobs

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"formlander/internal/forms"
	"formlander/internal/integrations"
	"formlander/internal/pkg/testsupport"
)

// pendingWebhook stores a form that delivers to url and one pending webhook.
func pendingWebhook(t *testing.T, db *gorm.DB, url string) *forms.WebhookEvent {
	t.Helper()
	profile := &integrations.WebhookProfile{Name: "Receiver", URL: url}
	require.NoError(t, db.Create(profile).Error)
	form := &forms.Form{Name: "Contact", Slug: "contact"}
	require.NoError(t, db.Create(form).Error)
	require.NoError(t, db.Create(&forms.WebhookDelivery{FormID: form.ID, Enabled: true, WebhookProfileID: &profile.ID}).Error)
	submission := &forms.Submission{FormID: form.ID, DataJSON: `{"name":"Ada"}`}
	require.NoError(t, db.Create(submission).Error)
	event := forms.NewWebhookEvent(submission.ID, time.Now().UTC().Add(-time.Minute))
	require.NoError(t, db.Create(event).Error)
	return event
}

func TestWebhookDeliveryClaim(t *testing.T) {
	jobCtx := func(db *gorm.DB) *JobContext {
		return &JobContext{Context: context.Background(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: db}
	}

	t.Run("sends a webhook once when two processes read it", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		var posts atomic.Int32
		receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			posts.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(receiver.Close)
		event := pendingWebhook(t, db, receiver.URL)
		dispatcher := NewWebhookDispatcher(webhookTestConfig())
		now := time.Now().UTC()
		oldContainer, err := dispatcher.due(db, now)
		require.NoError(t, err)
		newContainer, err := dispatcher.due(db, now)
		require.NoError(t, err)

		dispatcher.deliver(jobCtx(db), db, oldContainer, now)
		dispatcher.deliver(jobCtx(db), db, newContainer, now)

		assert.Equal(t, int32(1), posts.Load())
		var stored forms.WebhookEvent
		require.NoError(t, db.First(&stored, event.ID).Error)
		assert.Equal(t, forms.WebhookStatusDelivered, stored.Status)
	})

	t.Run("makes a claimed webhook due again when the process stops before it sends", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		event := pendingWebhook(t, db, "http://127.0.0.1:1")
		dispatcher := NewWebhookDispatcher(webhookTestConfig())
		now := time.Now().UTC()

		claimed, err := claim(db, &forms.WebhookEvent{}, event.ID, now)
		require.NoError(t, err)
		duringLease, err := dispatcher.due(db, now.Add(time.Minute))
		require.NoError(t, err)
		afterLease, err := dispatcher.due(db, now.Add(claimLease+time.Second))
		require.NoError(t, err)

		assert.True(t, claimed)
		assert.Empty(t, duringLease, "nobody else sends it while the lease runs")
		require.Len(t, afterLease, 1, "it is not lost")
		assert.Equal(t, event.ID, afterLease[0].ID)
	})
}
