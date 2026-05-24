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

// secFetchBlockedBody is cartridge's strict-middleware rejection body —
// any route returning this without a Sec-Fetch-Site header was blocked
// by CSRF protection.
const secFetchBlockedBody = "browser requests only"

// TestRoutesSecFetchSiteBoundary asserts which routes accept POSTs from
// clients that don't send the Sec-Fetch-Site header (older browsers,
// proxies that strip fetch-metadata, server-to-server) and which are
// still protected by cartridge's strict CSRF middleware.
//
// The two groups together describe the intended security boundary:
//
//	OPEN (no Sec-Fetch-Site required)
//	  - POST /admin/login              ← unauthenticated entry point
//	  - POST /forms/:slug/submit       ← public form ingestion (token + origin allowlist)
//
//	PROTECTED (Sec-Fetch-Site required for state-changing requests)
//	  - POST /admin/logout
//	  - POST /admin/forms
//	  - POST /admin/settings/password
//
// If a new state-changing admin route is added, add it to the protected
// group below to prevent it from being accidentally exposed.
func TestRoutesSecFetchSiteBoundary(t *testing.T) {
	// Silence cartridge's default slog during tests.
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	openRoutes := []struct {
		name string
		path string
		body string
	}{
		{"POST /admin/login (issue #35)", "/admin/login", "email=admin@formlander.local&password=formlander"},
		{"POST /forms/:slug/submit (public form)", "/forms/does-not-exist/submit?token=x", "field=value"},
	}

	protectedRoutes := []struct {
		name string
		path string
		body string
	}{
		{"POST /admin/logout", "/admin/logout", ""},
		{"POST /admin/forms", "/admin/forms", "name=test"},
		{"POST /admin/settings/password", "/admin/settings/password", ""},
	}

	t.Run("OPEN: accept POST without Sec-Fetch-Site", func(t *testing.T) {
		for _, r := range openRoutes {
			t.Run(r.name, func(t *testing.T) {
				ts := mountTestServer(t)
				seedAdmin(t, ts, "admin@formlander.local", "formlander")

				status, body := formPost(t, ts, r.path, r.body, nil)

				assert.NotEqual(t, 403, status,
					"route is opted out of Sec-Fetch-Site but returned 403")
				assert.NotContains(t, body, secFetchBlockedBody,
					"route was rejected by cartridge's strict SecFetchSite middleware")
			})
		}
	})

	t.Run("PROTECTED: reject POST without Sec-Fetch-Site", func(t *testing.T) {
		for _, r := range protectedRoutes {
			t.Run(r.name, func(t *testing.T) {
				ts := mountTestServer(t)

				status, body := formPost(t, ts, r.path, r.body, nil)

				assert.Equal(t, 403, status,
					"protected route must reject requests missing Sec-Fetch-Site")
				assert.Contains(t, body, secFetchBlockedBody,
					"rejection must come from SecFetchSite middleware, not the handler")
			})
		}
	})

	t.Run("login accepts POST with Sec-Fetch-Site: same-origin", func(t *testing.T) {
		ts := mountTestServer(t)
		seedAdmin(t, ts, "admin@formlander.local", "formlander")

		status, _ := formPost(t, ts, "/admin/login",
			"email=admin@formlander.local&password=formlander",
			map[string]string{"Sec-Fetch-Site": "same-origin"})

		assert.Equal(t, 302, status)
	})
}

// TestPublicFormSubmissionGuards covers the protections the public ingestion
// endpoint relies on instead of Sec-Fetch-Site: per-form token and the
// per-form Origin/Referer allowlist. If either guard regresses, an attacker
// could either replay submissions cross-site or post without knowing the
// form's secret.
func TestPublicFormSubmissionGuards(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	seedForm := func(t *testing.T, ts *cartridgetestsupport.TestServer) *forms.Form {
		t.Helper()
		f := &forms.Form{
			Name:           "Contact",
			Slug:           "contact",
			Token:          "secret-token",
			AllowedOrigins: "example.com",
		}
		require.NoError(t, ts.DB.GetConnection().Create(f).Error)
		return f
	}

	t.Run("rejects request with missing token", func(t *testing.T) {
		ts := mountTestServer(t)
		seedForm(t, ts)

		status, _ := formPost(t, ts, "/forms/contact/submit", "field=value",
			map[string]string{"Origin": "https://example.com"})

		assert.Equal(t, 401, status)
	})

	t.Run("rejects request with wrong token", func(t *testing.T) {
		ts := mountTestServer(t)
		seedForm(t, ts)

		status, _ := formPost(t, ts, "/forms/contact/submit?token=wrong", "field=value",
			map[string]string{"Origin": "https://example.com"})

		assert.Equal(t, 401, status)
	})

	t.Run("rejects request from origin not in allowlist", func(t *testing.T) {
		ts := mountTestServer(t)
		seedForm(t, ts)

		status, body := formPost(t, ts, "/forms/contact/submit?token=secret-token", "field=value",
			map[string]string{"Origin": "https://attacker.com"})

		assert.Equal(t, 403, status)
		assert.Contains(t, body, "origin not allowed")
	})

	t.Run("rejects request with no Origin or Referer when allowlist is set", func(t *testing.T) {
		ts := mountTestServer(t)
		seedForm(t, ts)

		status, body := formPost(t, ts, "/forms/contact/submit?token=secret-token", "field=value", nil)

		assert.Equal(t, 403, status)
		assert.Contains(t, body, "origin not allowed")
	})

	t.Run("accepts request with valid token and allowed origin", func(t *testing.T) {
		ts := mountTestServer(t)
		seedForm(t, ts)

		status, _ := formPost(t, ts, "/forms/contact/submit?token=secret-token", "field=value",
			map[string]string{"Origin": "https://example.com"})

		assert.Equal(t, 200, status)
	})
}
