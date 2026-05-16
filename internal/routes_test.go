package internal_test

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/karloscodes/cartridge"
	cartridgeconfig "github.com/karloscodes/cartridge/config"
	cartridgetestsupport "github.com/karloscodes/cartridge/testsupport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"formlander/internal"
	"formlander/internal/accounts"
	"formlander/internal/config"
	"formlander/internal/forms"
	"formlander/internal/integrations"
)

// Smoke tests for the Sec-Fetch-Site boundary on /admin routes.
//
// Cartridge's SecFetchSiteMiddleware rejects POSTs missing the
// Sec-Fetch-Site header (issue #35). Login is opted out so older
// browsers and reverse-proxied deploys can authenticate; every other
// state-changing admin route must remain protected.

func mountTestServer(t *testing.T) *cartridgetestsupport.TestServer {
	t.Helper()

	models := []any{
		&accounts.User{},
		&forms.Form{},
		&forms.Submission{},
		&forms.EmailDelivery{},
		&forms.WebhookDelivery{},
		&forms.WebhookEvent{},
		&forms.EmailEvent{},
		&integrations.MailerProfile{},
		&integrations.CaptchaProfile{},
	}

	flCfg := &config.Config{
		Config: &cartridgeconfig.Config{
			AppName:        "formlander",
			Environment:    cartridgeconfig.Test,
			SessionSecret:  "test-secret",
			SessionTimeout: 3600,
		},
		MaxInputFields: 200,
	}

	ts := cartridgetestsupport.NewTestServer(t, cartridgetestsupport.TestServerOptions{
		Models: models,
		RouteMountFunc: func(s *cartridge.Server) {
			s.SetSession(cartridge.NewSessionManager(cartridge.SessionConfig{
				CookieName: "formlander_session",
				Secret:     "test-secret",
				TTL:        time.Hour,
				LoginPath:  "/admin/login",
			}))
			internal.MountRoutes(s, flCfg)
		},
	})

	return ts
}

func formPost(t *testing.T, ts *cartridgetestsupport.TestServer, path, body string, headers map[string]string) (status int, respBody string) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ts.App.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func seedAdmin(t *testing.T, ts *cartridgetestsupport.TestServer, email, password string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)
	now := time.Now()
	user := &accounts.User{
		Email:        email,
		PasswordHash: string(hash),
		LastLoginAt:  &now,
	}
	require.NoError(t, ts.DB.GetConnection().Create(user).Error)
}

func TestRoutesSecFetchSiteBoundary(t *testing.T) {
	// Silence cartridge's default slog during tests.
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	t.Run("login accepts POST without Sec-Fetch-Site (issue #35)", func(t *testing.T) {
		ts := mountTestServer(t)
		seedAdmin(t, ts, "admin@formlander.local", "formlander")

		status, body := formPost(t, ts, "/admin/login",
			"email=admin@formlander.local&password=formlander", nil)

		assert.Equal(t, 302, status, "login must succeed without Sec-Fetch-Site")
		assert.NotContains(t, body, "browser requests only",
			"login must not be rejected by SecFetchSite middleware")
	})

	t.Run("login accepts POST with Sec-Fetch-Site: same-origin", func(t *testing.T) {
		ts := mountTestServer(t)
		seedAdmin(t, ts, "admin@formlander.local", "formlander")

		status, _ := formPost(t, ts, "/admin/login",
			"email=admin@formlander.local&password=formlander",
			map[string]string{"Sec-Fetch-Site": "same-origin"})

		assert.Equal(t, 302, status)
	})

	t.Run("logout rejects POST without Sec-Fetch-Site (still protected)", func(t *testing.T) {
		ts := mountTestServer(t)
		status, body := formPost(t, ts, "/admin/logout", "", nil)
		assert.Equal(t, 403, status)
		assert.Contains(t, body, "browser requests only")
	})

	t.Run("change-password rejects POST without Sec-Fetch-Site (still protected)", func(t *testing.T) {
		ts := mountTestServer(t)
		status, body := formPost(t, ts, "/admin/change-password",
			"current_password=x&new_password=y&confirm_password=y", nil)
		assert.Equal(t, 403, status)
		assert.Contains(t, body, "browser requests only")
	})

	t.Run("forms create rejects POST without Sec-Fetch-Site (still protected)", func(t *testing.T) {
		ts := mountTestServer(t)
		status, body := formPost(t, ts, "/admin/forms", "name=test", nil)
		assert.Equal(t, 403, status)
		assert.Contains(t, body, "browser requests only")
	})

	t.Run("settings password rejects POST without Sec-Fetch-Site (still protected)", func(t *testing.T) {
		ts := mountTestServer(t)
		status, body := formPost(t, ts, "/admin/settings/password", "", nil)
		assert.Equal(t, 403, status)
		assert.Contains(t, body, "browser requests only")
	})

	t.Run("public form submission allows missing Sec-Fetch-Site (unchanged)", func(t *testing.T) {
		ts := mountTestServer(t)
		// No form exists with this slug; we only care the SecFetchSite
		// middleware does NOT block — a 404 from the handler is fine.
		status, body := formPost(t, ts, "/forms/does-not-exist/submit?token=x", "field=value", nil)
		assert.NotEqual(t, 403, status, "public ingestion must not require Sec-Fetch-Site")
		assert.NotContains(t, body, "browser requests only")
	})
}
