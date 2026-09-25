package http

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"formlander/internal/forms"
	"formlander/internal/pkg/testsupport"
)

// oldWaitlistHTML is the start of a waitlist template saved before v3.1.2.
const oldWaitlistHTML = `
<div class="formlander-shell">
    <div class="formlander-eyebrow" style="background: rgba(245, 158, 11, 0.16); color: #d97706;">Waitlist</div>
    <h2>Join the early access list</h2>
    <form action="/forms/waitlist/submit?token=abc" method="POST" class="formlander-stack">
        <button type="submit" class="formlander-button">Request invite</button>
    </form>
</div>
<style>
    .formlander-button { background: linear-gradient(135deg, #2563eb, #4338ca); }
</style>`

func TestUpgradeOldStarterHTML(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("replaces an old template with the current one", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		form := &forms.Form{Name: "Waitlist", Slug: "waitlist", Token: "abc", GeneratedHTML: oldWaitlistHTML}
		require.NoError(t, db.Create(form).Error)

		err := UpgradeOldStarterHTML(logger, db)

		require.NoError(t, err)
		upgraded, err := forms.GetByID(db, form.ID)
		require.NoError(t, err)
		assert.Contains(t, upgraded.GeneratedHTML, "Join the waitlist")
		assert.Contains(t, upgraded.GeneratedHTML, "/forms/waitlist/submit?token=abc")
		assert.NotContains(t, upgraded.GeneratedHTML, oldTemplateMarker)
	})

	t.Run("leaves HTML without the old marker alone", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		custom := `<form action="/forms/custom/submit"><input name="email"></form>`
		form := &forms.Form{Name: "Custom", Slug: "custom", Token: "abc", GeneratedHTML: custom}
		require.NoError(t, db.Create(form).Error)

		err := UpgradeOldStarterHTML(logger, db)

		require.NoError(t, err)
		unchanged, err := forms.GetByID(db, form.ID)
		require.NoError(t, err)
		assert.Equal(t, custom, unchanged.GeneratedHTML)
	})
}
