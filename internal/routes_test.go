package internal_test

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// testSessionSecret signs the sessions of the test server.
const testSessionSecret = "a-test-session-secret-of-32-bytes-or-more"

func mountTestServer(t *testing.T) *cartridgetestsupport.TestServer {
	t.Helper()

	models := []any{
		&accounts.User{},
		&accounts.EndedSession{},
		&forms.Form{},
		&forms.Submission{},
		&forms.EmailDelivery{},
		&forms.WebhookDelivery{},
		&forms.WebhookEvent{},
		&forms.EmailEvent{},
		&forms.SubmissionFile{},
		&integrations.MailerProfile{},
		&integrations.CaptchaProfile{},
		&integrations.WebhookProfile{},
	}

	flCfg := &config.Config{
		Config: &cartridgeconfig.Config{
			AppName:        "formlander",
			Environment:    cartridgeconfig.Test,
			SessionSecret:  testSessionSecret,
			SessionTimeout: 3600,
			DataDirectory:  t.TempDir(),
		},
		MaxInputFields: 200,
	}

	// The session check uses the database of the server, which exists once
	// NewTestServer returns; the check runs only during requests.
	var ts *cartridgetestsupport.TestServer
	ts = cartridgetestsupport.NewTestServer(t, cartridgetestsupport.TestServerOptions{
		Models: models,
		RouteMountFunc: func(s *cartridge.Server) {
			sessions, err := cartridge.NewSessionManager(cartridge.SessionConfig{
				CookieName: "formlander_session",
				Secret:     testSessionSecret,
				TTL:        time.Hour,
				LoginPath:  "/admin/login",
				Insecure:   true,
				Valid: func(userID uint, issuedAt time.Time) bool {
					return accounts.SessionValid(ts.DB.GetConnection(), userID, issuedAt)
				},
			})
			require.NoError(t, err)
			s.SetSession(sessions)
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
	resp, err := ts.Server.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func multipartPost(t *testing.T, ts *cartridgetestsupport.TestServer, path string, fields map[string]string, fileName string, headers map[string]string) (status int, respBody string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		require.NoError(t, w.WriteField(k, v))
	}
	if fileName != "" {
		fw, err := w.CreateFormFile("attachment", fileName)
		require.NoError(t, err)
		_, err = fw.Write([]byte("hello"))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ts.Server.Test(req)
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
//	  - the deletes and the "Send a test" actions
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
		{"POST /admin/submissions/:id/delete", "/admin/submissions/1/delete", ""},
		{"POST /admin/submissions/delete", "/admin/submissions/delete", "ids=1"},
		{"POST /admin/settings/webhooks", "/admin/settings/webhooks", "name=test"},
		{"POST /admin/settings/webhooks/:id/delete", "/admin/settings/webhooks/1/delete", ""},
		{"POST /admin/settings/webhooks/:id/test", "/admin/settings/webhooks/1/test", ""},
		{"POST /admin/settings/mailers/:id/test", "/admin/settings/mailers/1/test", ""},
		{"POST /admin/settings/captcha/:id/test", "/admin/settings/captcha/1/test", ""},
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
		assert.Contains(t, body, "origin not allowed: add attacker.com to this form's Allowed Origins")
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

	t.Run("accepts a multipart post that holds only a file", func(t *testing.T) {
		ts := mountTestServer(t)
		seedForm(t, ts)

		status, body := multipartPost(t, ts, "/forms/contact/submit?token=secret-token", nil, "notes.txt",
			map[string]string{"Origin": "https://example.com"})

		assert.Equal(t, 200, status, body)
	})

	t.Run("redirects to _error_url when the payload is rejected", func(t *testing.T) {
		ts := mountTestServer(t)
		seedForm(t, ts)
		fields := "_error_url=https%3A%2F%2Fexample.com%2Foops"
		for i := 0; i < 250; i++ {
			fields += fmt.Sprintf("&f%d=x", i)
		}

		req := httptest.NewRequest("POST", "/forms/contact/submit?token=secret-token", strings.NewReader(fields))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "https://example.com")
		resp, err := ts.Server.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 302, resp.StatusCode)
		assert.Equal(t, "https://example.com/oops", resp.Header.Get("Location"))
	})

	t.Run("falls back to Referer when the browser sends Origin: null", func(t *testing.T) {
		ts := mountTestServer(t)
		seedForm(t, ts)

		status, _ := formPost(t, ts, "/forms/contact/submit?token=secret-token", "field=value",
			map[string]string{"Origin": "null", "Referer": "https://example.com/contact"})

		assert.Equal(t, 200, status)
	})

	t.Run("redirects a browser to the thank-you page, so a refresh does not post again", func(t *testing.T) {
		ts := mountTestServer(t)
		seedForm(t, ts)
		req := httptest.NewRequest("POST", "/forms/contact/submit?token=secret-token", strings.NewReader("field=value"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Sec-Fetch-Mode", "navigate")

		resp, err := ts.Server.Test(req)

		require.NoError(t, err)
		assert.Equal(t, 303, resp.StatusCode)
		assert.Equal(t, "/forms/sent", resp.Header.Get("Location"))
	})

	t.Run("the thank-you page goes back through the browser history", func(t *testing.T) {
		ts := mountTestServer(t)
		req := httptest.NewRequest("GET", "/forms/sent", nil)
		req.Header.Set("Referer", "https://example.com/")

		resp, err := ts.Server.Test(req)

		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Contains(t, string(body), "Thanks, we got it.")
		assert.Contains(t, string(body), "history.back()")
	})

	t.Run("shows an error page to a browser posting from a site not allowed", func(t *testing.T) {
		ts := mountTestServer(t)
		seedForm(t, ts)

		status, body := formPost(t, ts, "/forms/contact/submit?token=secret-token", "field=value",
			map[string]string{"Origin": "https://attacker.com", "Sec-Fetch-Mode": "navigate"})

		assert.Equal(t, 403, status)
		assert.Contains(t, body, "This form can&#39;t be sent from this site.")
		assert.Contains(t, body, "add attacker.com to this form&#39;s Allowed Origins")
		assert.Contains(t, body, "history.back()", "Go back returns to the form, not the site's home page")
	})
}

func TestSessionsEndAfterPasswordChange(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	login := func(t *testing.T, ts *cartridgetestsupport.TestServer, password string) []*http.Cookie {
		t.Helper()
		req := httptest.NewRequest("POST", "/admin/login", strings.NewReader("email=admin@example.com&password="+password))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := ts.Server.Test(req)
		require.NoError(t, err)
		require.Equal(t, 302, resp.StatusCode)
		return resp.Cookies()
	}
	// The test server has no templates, so a signed-in request fails to
	// render. Only the redirect to the login page shows a signed-out session.
	signedIn := func(t *testing.T, ts *cartridgetestsupport.TestServer, cookies []*http.Cookie) bool {
		t.Helper()
		req := httptest.NewRequest("GET", "/admin", nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		resp, err := ts.Server.Test(req)
		require.NoError(t, err)
		return resp.Header.Get("Location") != "/admin/login"
	}

	t.Run("an operator reset signs out existing sessions", func(t *testing.T) {
		ts := mountTestServer(t)
		seedAdmin(t, ts, "admin@example.com", "old-password-1")
		cookies := login(t, ts, "old-password-1")
		require.True(t, signedIn(t, ts, cookies))

		err := accounts.ResetPassword(slog.Default(), ts.DB.GetConnection(), "admin@example.com", "new-password-1")
		require.NoError(t, err)

		assert.False(t, signedIn(t, ts, cookies))
	})

	t.Run("a new login after the change works", func(t *testing.T) {
		ts := mountTestServer(t)
		seedAdmin(t, ts, "admin@example.com", "old-password-1")
		require.NoError(t, accounts.ResetPassword(slog.Default(), ts.DB.GetConnection(), "admin@example.com", "new-password-1"))

		cookies := login(t, ts, "new-password-1")

		assert.True(t, signedIn(t, ts, cookies))
	})
}

func TestSignedOutRequestsGoToLogin(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	get := func(t *testing.T, ts *cartridgetestsupport.TestServer, headers map[string]string, cookies []*http.Cookie) *http.Response {
		t.Helper()
		req := httptest.NewRequest("GET", "/admin/submissions", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		resp, err := ts.Server.Test(req)
		require.NoError(t, err)
		return resp
	}

	t.Run("redirects a page load without a session", func(t *testing.T) {
		ts := mountTestServer(t)

		resp := get(t, ts, nil, nil)

		assert.Equal(t, 302, resp.StatusCode)
		assert.Equal(t, "/admin/login", resp.Header.Get("Location"))
	})

	t.Run("tells htmx to load the login page for a click without a session", func(t *testing.T) {
		ts := mountTestServer(t)

		resp := get(t, ts, map[string]string{"HX-Request": "true"}, nil)

		assert.Equal(t, 401, resp.StatusCode)
		assert.Equal(t, "/admin/login", resp.Header.Get("HX-Redirect"))
	})

	t.Run("tells htmx to load the login page for a click with a session that ended", func(t *testing.T) {
		ts := mountTestServer(t)
		seedAdmin(t, ts, "admin@example.com", "old-password-1")
		req := httptest.NewRequest("POST", "/admin/login", strings.NewReader("email=admin@example.com&password=old-password-1"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		login, err := ts.Server.Test(req)
		require.NoError(t, err)
		require.NoError(t, accounts.ResetPassword(slog.Default(), ts.DB.GetConnection(), "admin@example.com", "new-password-1"))

		resp := get(t, ts, map[string]string{"HX-Request": "true"}, login.Cookies())

		assert.Equal(t, 401, resp.StatusCode)
		assert.Equal(t, "/admin/login", resp.Header.Get("HX-Redirect"))
	})
}

func TestLogoutEndsTheSession(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	login := func(t *testing.T, ts *cartridgetestsupport.TestServer) []*http.Cookie {
		t.Helper()
		req := httptest.NewRequest("POST", "/admin/login", strings.NewReader("email=admin@example.com&password=a-good-password"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := ts.Server.Test(req)
		require.NoError(t, err)
		require.Equal(t, 302, resp.StatusCode)
		return resp.Cookies()
	}
	logout := func(t *testing.T, ts *cartridgetestsupport.TestServer, cookies []*http.Cookie) {
		t.Helper()
		req := httptest.NewRequest("POST", "/admin/logout", nil)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		resp, err := ts.Server.Test(req)
		require.NoError(t, err)
		require.Equal(t, "/admin/login", resp.Header.Get("Location"))
	}
	// The test server has no templates, so a signed-in request fails to
	// render. Only the redirect to the login page shows a signed-out session.
	signedIn := func(t *testing.T, ts *cartridgetestsupport.TestServer, cookies []*http.Cookie) bool {
		t.Helper()
		req := httptest.NewRequest("GET", "/admin", nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		resp, err := ts.Server.Test(req)
		require.NoError(t, err)
		return resp.Header.Get("Location") != "/admin/login"
	}

	t.Run("a cookie copied before logout no longer works", func(t *testing.T) {
		ts := mountTestServer(t)
		seedAdmin(t, ts, "admin@example.com", "a-good-password")
		copied := login(t, ts)
		require.True(t, signedIn(t, ts, copied))

		logout(t, ts, copied)

		assert.False(t, signedIn(t, ts, copied))
	})

	t.Run("logout on one device keeps the other device signed in", func(t *testing.T) {
		ts := mountTestServer(t)
		seedAdmin(t, ts, "admin@example.com", "a-good-password")
		laptop := login(t, ts)
		phone := login(t, ts)

		logout(t, ts, laptop)

		assert.False(t, signedIn(t, ts, laptop))
		assert.True(t, signedIn(t, ts, phone))
	})
}

// signIn logs the admin in and returns a function that posts a form as that
// admin, the way the admin pages do.
func signIn(t *testing.T, ts *cartridgetestsupport.TestServer) func(path, body string) *http.Response {
	t.Helper()
	seedAdmin(t, ts, "admin@example.com", "a-good-password")
	req := httptest.NewRequest("POST", "/admin/login", strings.NewReader("email=admin@example.com&password=a-good-password"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := ts.Server.Test(req)
	require.NoError(t, err)
	require.Equal(t, 302, resp.StatusCode)
	cookies := resp.Cookies()

	return func(path, body string) *http.Response {
		t.Helper()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		resp, err := ts.Server.Test(req)
		require.NoError(t, err)
		return resp
	}
}

func TestAdminDeletesSubmissions(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	// seed makes a form with two real submissions and one spam submission.
	seed := func(t *testing.T, ts *cartridgetestsupport.TestServer) (real []forms.Submission) {
		t.Helper()
		db := ts.DB.GetConnection()
		form := &forms.Form{Name: "Contact", Slug: "contact", AllowedOrigins: "example.com"}
		require.NoError(t, db.Create(form).Error)
		for _, payload := range []map[string]any{
			{"name": "Ada"},
			{"name": "Grace"},
			{"name": "Bot", forms.HoneypotField: "filled"},
		} {
			_, err := forms.CreateSubmission(slog.Default(), db, form, payload, "test")
			require.NoError(t, err)
		}
		require.NoError(t, db.Where("is_spam = ?", false).Order("id").Find(&real).Error)
		return real
	}
	remaining := func(t *testing.T, ts *cartridgetestsupport.TestServer) (ids []uint) {
		t.Helper()
		require.NoError(t, ts.DB.GetConnection().Model(&forms.Submission{}).Order("id").Pluck("id", &ids).Error)
		return ids
	}

	t.Run("deletes one submission and goes back to the list the owner was on", func(t *testing.T) {
		ts := mountTestServer(t)
		real := seed(t, ts)
		post := signIn(t, ts)

		resp := post(fmt.Sprintf("/admin/submissions/%d/delete", real[0].ID), "return_to=%2Fadmin%2Fsubmissions%3Fpage%3D2")

		assert.Equal(t, 302, resp.StatusCode)
		assert.Equal(t, "/admin/submissions?page=2", resp.Header.Get("Location"))
		assert.NotContains(t, remaining(t, ts), real[0].ID)
		assert.Len(t, remaining(t, ts), 2)
	})

	t.Run("does not follow a return address outside the admin", func(t *testing.T) {
		ts := mountTestServer(t)
		real := seed(t, ts)
		post := signIn(t, ts)

		resp := post(fmt.Sprintf("/admin/submissions/%d/delete", real[0].ID), "return_to=https%3A%2F%2Fevil.example%2F")

		assert.Equal(t, "/admin/submissions", resp.Header.Get("Location"))
	})

	t.Run("answers 404 for a submission that does not exist", func(t *testing.T) {
		ts := mountTestServer(t)
		post := signIn(t, ts)

		resp := post("/admin/submissions/999/delete", "")

		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("deletes the submissions that were ticked", func(t *testing.T) {
		ts := mountTestServer(t)
		real := seed(t, ts)
		post := signIn(t, ts)

		resp := post("/admin/submissions/delete", fmt.Sprintf("ids=%d&ids=%d&return_to=%%2Fadmin%%2Fforms%%2F1", real[0].ID, real[1].ID))

		assert.Equal(t, "/admin/forms/1", resp.Header.Get("Location"))
		assert.Len(t, remaining(t, ts), 1, "only the spam is left")
	})

	t.Run("deletes all spam and keeps the rest", func(t *testing.T) {
		ts := mountTestServer(t)
		real := seed(t, ts)
		post := signIn(t, ts)

		resp := post("/admin/submissions/delete", "spam=1")

		assert.Equal(t, 302, resp.StatusCode)
		assert.Equal(t, []uint{real[0].ID, real[1].ID}, remaining(t, ts))
	})

	t.Run("refuses a visitor who is not signed in", func(t *testing.T) {
		ts := mountTestServer(t)
		real := seed(t, ts)

		status, _ := formPost(t, ts, fmt.Sprintf("/admin/submissions/%d/delete", real[0].ID), "",
			map[string]string{"Sec-Fetch-Site": "same-origin"})

		assert.Equal(t, 302, status)
		assert.Len(t, remaining(t, ts), 3)
	})
}

func TestAdminWebhookProfiles(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	t.Run("saves the header rows of the form as the headers of the profile", func(t *testing.T) {
		ts := mountTestServer(t)
		post := signIn(t, ts)

		resp := post("/admin/settings/webhooks",
			"name=Zapier&url=https%3A%2F%2Fhooks.example.com%2Fin&secret=s3cret"+
				"&header_name=Authorization&header_value=Bearer+token"+
				"&header_name=&header_value="+
				"&header_name=X-Team&header_value=sales")

		assert.Equal(t, 302, resp.StatusCode)
		profiles, err := integrations.ListWebhookProfiles(ts.DB.GetConnection())
		require.NoError(t, err)
		require.Len(t, profiles, 1)
		assert.Equal(t, "/admin/settings/webhooks/"+fmt.Sprint(profiles[0].ID), resp.Header.Get("Location"))
		assert.Equal(t, []integrations.WebhookHeader{
			{Name: "Authorization", Value: "Bearer token"},
			{Name: "X-Team", Value: "sales"},
		}, profiles[0].Headers())
	})

	t.Run("the key field takes a key or a header line, as the service shows it", func(t *testing.T) {
		for pasted, want := range map[string]integrations.WebhookHeader{
			"crsr_abc123":                       {Name: "Authorization", Value: "Bearer crsr_abc123"},
			"Bearer crsr_abc123":                {Name: "Authorization", Value: "Bearer crsr_abc123"},
			"Authorization: Bearer crsr_abc123": {Name: "Authorization", Value: "Bearer crsr_abc123"},
			"Basic dXNlcjpwYXNz":                {Name: "Authorization", Value: "Basic dXNlcjpwYXNz"},
			"X-API-Key: abc123":                 {Name: "X-API-Key", Value: "abc123"},
			"user:password":                     {Name: "Authorization", Value: "Bearer user:password"},
		} {
			ts := mountTestServer(t)
			post := signIn(t, ts)

			resp := post("/admin/settings/webhooks", "name=Service&url=https%3A%2F%2Fhooks.example.com%2Fin"+
				"&key="+url.QueryEscape(pasted)+
				"&header_name="+url.QueryEscape(strings.ToLower(want.Name))+"&header_value=old"+
				"&header_name=X-Team&header_value=sales")

			require.Equal(t, 302, resp.StatusCode, pasted)
			profiles, err := integrations.ListWebhookProfiles(ts.DB.GetConnection())
			require.NoError(t, err)
			require.Len(t, profiles, 1)
			assert.Equal(t, []integrations.WebhookHeader{want, {Name: "X-Team", Value: "sales"}}, profiles[0].Headers(), pasted)
		}
	})

	t.Run("a webhook with the secret in its URL needs no key", func(t *testing.T) {
		ts := mountTestServer(t)
		post := signIn(t, ts)

		resp := post("/admin/settings/webhooks", "name=Zapier&url=https%3A%2F%2Fhooks.zapier.com%2Fhooks%2Fcatch%2F1%2Fabc&key=")

		require.Equal(t, 302, resp.StatusCode)
		profiles, err := integrations.ListWebhookProfiles(ts.DB.GetConnection())
		require.NoError(t, err)
		require.Len(t, profiles, 1)
		assert.Empty(t, profiles[0].Headers())
	})

	t.Run("deletes a profile that no form uses", func(t *testing.T) {
		ts := mountTestServer(t)
		db := ts.DB.GetConnection()
		profile := &integrations.WebhookProfile{Name: "Zapier", URL: "https://hooks.example.com/in"}
		require.NoError(t, db.Create(profile).Error)
		post := signIn(t, ts)

		resp := post(fmt.Sprintf("/admin/settings/webhooks/%d/delete", profile.ID), "")

		assert.Equal(t, "/admin/settings/webhooks", resp.Header.Get("Location"))
		profiles, err := integrations.ListWebhookProfiles(db)
		require.NoError(t, err)
		assert.Empty(t, profiles)
	})

	t.Run("keeps a profile that a form uses", func(t *testing.T) {
		ts := mountTestServer(t)
		db := ts.DB.GetConnection()
		profile := &integrations.WebhookProfile{Name: "Zapier", URL: "https://hooks.example.com/in"}
		require.NoError(t, db.Create(profile).Error)
		form := &forms.Form{Name: "Contact", Slug: "contact"}
		require.NoError(t, db.Create(form).Error)
		require.NoError(t, db.Create(&forms.WebhookDelivery{FormID: form.ID, Enabled: true, WebhookProfileID: &profile.ID}).Error)
		post := signIn(t, ts)

		resp := post(fmt.Sprintf("/admin/settings/webhooks/%d/delete", profile.ID), "")

		assert.NotEqual(t, 302, resp.StatusCode)
		profiles, err := integrations.ListWebhookProfiles(db)
		require.NoError(t, err)
		assert.Len(t, profiles, 1)
	})

	t.Run("a form picks a webhook profile", func(t *testing.T) {
		ts := mountTestServer(t)
		db := ts.DB.GetConnection()
		profile := &integrations.WebhookProfile{Name: "Zapier", URL: "https://hooks.example.com/in"}
		require.NoError(t, db.Create(profile).Error)
		post := signIn(t, ts)

		resp := post("/admin/forms", fmt.Sprintf("name=Contact&slug=contact&allowed_origins=mysite.test&webhook_enabled=on&webhook_profile_id=%d", profile.ID))

		require.Equal(t, 302, resp.StatusCode)
		form, err := forms.GetBySlug(db, "contact")
		require.NoError(t, err)
		assert.True(t, form.WebhookDelivery.Delivers())
		assert.Equal(t, "Zapier", form.WebhookDelivery.WebhookProfile.Name)
	})
}

func TestAdminCaptchaProfileFields(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	onlyProfile := func(t *testing.T, ts *cartridgetestsupport.TestServer) integrations.CaptchaProfile {
		t.Helper()
		profiles, err := integrations.ListCaptchaProfiles(ts.DB.GetConnection())
		require.NoError(t, err)
		require.Len(t, profiles, 1)
		return profiles[0]
	}

	t.Run("one site key covers every site", func(t *testing.T) {
		ts := mountTestServer(t)
		post := signIn(t, ts)

		resp := post("/admin/settings/captcha", "name=Site&provider=turnstile&secret_key=secret&site_key=0xMAIN&host_pattern=&theme=auto")

		require.Equal(t, 302, resp.StatusCode)
		profile := onlyProfile(t, ts)
		assert.JSONEq(t, `[{"host_pattern":"*","site_key":"0xMAIN"}]`, profile.SiteKeysJSON)
		assert.Empty(t, profile.PolicyJSON)
	})

	t.Run("rows give one site key for each domain, and the widget options are saved", func(t *testing.T) {
		ts := mountTestServer(t)
		post := signIn(t, ts)

		resp := post("/admin/settings/captcha", "name=Sites&provider=turnstile&secret_key=secret"+
			"&site_key=0xMAIN&host_pattern=example.com"+
			"&host_pattern=*.example.org&site_key=0xORG"+
			"&host_pattern=&site_key="+
			"&theme=dark&size=compact&language=es&action=signup")

		require.Equal(t, 302, resp.StatusCode)
		profile := onlyProfile(t, ts)
		assert.JSONEq(t, `[{"host_pattern":"example.com","site_key":"0xMAIN"},{"host_pattern":"*.example.org","site_key":"0xORG"}]`, profile.SiteKeysJSON)
		assert.JSONEq(t, `{"theme":"dark","size":"compact","language":"es","action":"signup"}`, profile.PolicyJSON)
	})

	t.Run("an update keeps the saved secret and the options the form does not show", func(t *testing.T) {
		ts := mountTestServer(t)
		db := ts.DB.GetConnection()
		saved := &integrations.CaptchaProfile{Name: "Site", Provider: "turnstile", SecretKey: "secret", PolicyJSON: `{"widget":"invisible","theme":"dark"}`}
		require.NoError(t, db.Create(saved).Error)
		post := signIn(t, ts)

		resp := post(fmt.Sprintf("/admin/settings/captcha/%d", saved.ID), "name=Site&provider=turnstile&secret_key=&site_key=0xMAIN&host_pattern=&theme=light&size=&language=&action=")

		require.Equal(t, 302, resp.StatusCode)
		profile := onlyProfile(t, ts)
		assert.Equal(t, "secret", profile.SecretKey)
		assert.JSONEq(t, `{"widget":"invisible","theme":"light"}`, profile.PolicyJSON)
	})

	t.Run("still takes the JSON fields of the old form", func(t *testing.T) {
		ts := mountTestServer(t)
		post := signIn(t, ts)

		resp := post("/admin/settings/captcha", "name=Old&provider=turnstile&secret_key=secret"+
			"&site_keys_json=%5B%7B%22host_pattern%22%3A%22a.com%22%2C%22site_key%22%3A%220xA%22%7D%5D"+
			"&policy_json=%7B%22theme%22%3A%22dark%22%7D")

		require.Equal(t, 302, resp.StatusCode)
		profile := onlyProfile(t, ts)
		assert.JSONEq(t, `[{"host_pattern":"a.com","site_key":"0xA"}]`, profile.SiteKeysJSON)
		assert.JSONEq(t, `{"theme":"dark"}`, profile.PolicyJSON)
	})
}

func TestAdminMailerProfileKeepsSecrets(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ts := mountTestServer(t)
	db := ts.DB.GetConnection()
	saved := &integrations.MailerProfile{
		Name: "Relay", Provider: "smtp", DefaultFromEmail: "forms@example.com",
		SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPUsername: "user", SMTPPassword: "pass",
		DefaultsJSON: `{"tags":["a"]}`,
	}
	require.NoError(t, db.Create(saved).Error)
	post := signIn(t, ts)

	resp := post(fmt.Sprintf("/admin/settings/mailers/%d", saved.ID),
		"name=Relay&provider=smtp&default_from_email=new%40example.com&smtp_host=smtp.example.com&smtp_port=465&smtp_username=user&smtp_password=&api_key=")

	require.Equal(t, 302, resp.StatusCode)
	profile, err := integrations.GetMailerProfileByID(db, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "new@example.com", profile.DefaultFromEmail)
	assert.Equal(t, 465, profile.SMTPPort)
	assert.Equal(t, "pass", profile.SMTPPassword, "an empty password field keeps the saved password")
	assert.Equal(t, `{"tags":["a"]}`, profile.DefaultsJSON)
}

func TestAdminExportsSubmissions(t *testing.T) {
	ts := mountTestServer(t)
	db := ts.DB.GetConnection()
	form := &forms.Form{Name: "Contact", Slug: "contact", AllowedOrigins: "example.com"}
	require.NoError(t, db.Create(form).Error)
	other := &forms.Form{Name: "Newsletter", Slug: "newsletter", AllowedOrigins: "example.com"}
	require.NoError(t, db.Create(other).Error)
	for _, entry := range []struct {
		form    *forms.Form
		payload map[string]any
	}{
		{form, map[string]any{"email": "ana@example.com", "message": "Hello, \"you\""}},
		{form, map[string]any{"email": "bob@example.com", "company": "=HYPERLINK(\"http://evil\")"}},
		{other, map[string]any{"email": "cy@example.com"}},
	} {
		_, err := forms.CreateSubmission(slog.Default(), db, entry.form, entry.payload, "test")
		require.NoError(t, err)
	}
	get := signInGet(t, ts)

	t.Run("one row for each submission, one column for each field", func(t *testing.T) {
		resp := get("/admin/submissions/export.csv")

		require.Equal(t, 200, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")
		rows, err := csv.NewReader(resp.Body).ReadAll()
		require.NoError(t, err)
		require.Len(t, rows, 4)
		assert.Equal(t, []string{"received", "form", "spam", "company", "email", "message"}, rows[0])
	})

	t.Run("the filters of the list apply", func(t *testing.T) {
		resp := get(fmt.Sprintf("/admin/submissions/export.csv?form_id=%d&q=ana", form.ID))

		rows, err := csv.NewReader(resp.Body).ReadAll()
		require.NoError(t, err)
		require.Len(t, rows, 2)
		assert.Equal(t, []string{"received", "form", "spam", "email", "message"}, rows[0])
		assert.Equal(t, []string{"Contact", "no", "ana@example.com", `Hello, "you"`}, rows[1][1:])
	})

	t.Run("a value cannot be a spreadsheet formula", func(t *testing.T) {
		resp := get("/admin/submissions/export.csv?q=bob")

		rows, err := csv.NewReader(resp.Body).ReadAll()
		require.NoError(t, err)
		require.Len(t, rows, 2)
		assert.Equal(t, `'=HYPERLINK("http://evil")`, rows[1][3])
	})

	t.Run("the spam choice keeps the spam, or leaves it out", func(t *testing.T) {
		require.NoError(t, db.Model(&forms.Submission{}).Where("data_json LIKE ?", "%cy@example.com%").Update("is_spam", true).Error)

		only, err := csv.NewReader(get("/admin/submissions/export.csv?spam=only").Body).ReadAll()
		require.NoError(t, err)
		without, err := csv.NewReader(get("/admin/submissions/export.csv?spam=no").Body).ReadAll()
		require.NoError(t, err)

		require.Len(t, only, 2)
		assert.Equal(t, "yes", only[1][2])
		assert.Len(t, without, 3)
	})

	t.Run("needs a login", func(t *testing.T) {
		resp, err := ts.Server.Test(httptest.NewRequest("GET", "/admin/submissions/export.csv", nil))

		require.NoError(t, err)
		assert.NotEqual(t, 200, resp.StatusCode)
	})
}

// signInGet signs the admin in and returns a function that gets a page.
func TestMailerProfileSendsItsPasswordOnlyToItsServer(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	setup := func(t *testing.T) (*cartridgetestsupport.TestServer, *integrations.MailerProfile, func(path, body string) *http.Response) {
		t.Helper()
		ts := mountTestServer(t)
		profile := &integrations.MailerProfile{
			Name: "Relay", Provider: "smtp", DefaultFromEmail: "forms@example.com",
			SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPUsername: "u", SMTPPassword: "dummy-secret", SMTPEncryption: "starttls",
		}
		require.NoError(t, ts.DB.GetConnection().Create(profile).Error)
		return ts, profile, signIn(t, ts)
	}
	update := func(host, password string) string {
		return url.Values{
			"name": {"Relay"}, "provider": {"smtp"}, "default_from_email": {"forms@example.com"},
			"smtp_host": {host}, "smtp_port": {"587"}, "smtp_username": {"u"}, "smtp_password": {password}, "smtp_encryption": {"starttls"},
		}.Encode()
	}
	saved := func(t *testing.T, ts *cartridgetestsupport.TestServer, id uint) integrations.MailerProfile {
		t.Helper()
		var profile integrations.MailerProfile
		require.NoError(t, ts.DB.GetConnection().First(&profile, id).Error)
		return profile
	}

	t.Run("refuses a new host when the password is not typed again", func(t *testing.T) {
		ts, profile, post := setup(t)

		post(fmt.Sprintf("/admin/settings/mailers/%d", profile.ID), update("smtp.attacker.example", ""))

		after := saved(t, ts, profile.ID)
		assert.Equal(t, "smtp.example.com", after.SMTPHost)
		assert.Equal(t, "dummy-secret", after.SMTPPassword)
	})

	t.Run("keeps the saved password when the server stays the same", func(t *testing.T) {
		ts, profile, post := setup(t)

		resp := post(fmt.Sprintf("/admin/settings/mailers/%d", profile.ID), update("smtp.example.com", ""))

		assert.Equal(t, 302, resp.StatusCode)
		assert.Equal(t, "dummy-secret", saved(t, ts, profile.ID).SMTPPassword)
	})

	t.Run("takes a new host with the password typed again", func(t *testing.T) {
		ts, profile, post := setup(t)

		resp := post(fmt.Sprintf("/admin/settings/mailers/%d", profile.ID), update("smtp.other.example", "new-secret"))

		assert.Equal(t, 302, resp.StatusCode)
		after := saved(t, ts, profile.ID)
		assert.Equal(t, "smtp.other.example", after.SMTPHost)
		assert.Equal(t, "new-secret", after.SMTPPassword)
	})
}

func TestAdminDownloadsAFile(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	ts := mountTestServer(t)
	db := ts.DB.GetConnection()
	require.NoError(t, db.Create(&forms.Form{Name: "Contact", Slug: "contact", Token: "secret-token", AllowedOrigins: "example.com"}).Error)
	name := `a";filename*=UTF-8''invoice.exe;x=".pdf`
	status, body := multipartPost(t, ts, "/forms/contact/submit?token=secret-token", nil, name,
		map[string]string{"Origin": "https://example.com"})
	require.Equal(t, 200, status, body)
	var file forms.SubmissionFile
	require.NoError(t, db.First(&file).Error)
	get := signInGet(t, ts)

	resp := get(fmt.Sprintf("/admin/submissions/%d/files/%d", file.SubmissionID, file.ID))

	require.Equal(t, 200, resp.StatusCode)
	disposition, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	require.NoError(t, err)
	assert.Equal(t, "attachment", disposition)
	assert.Equal(t, map[string]string{"filename": name}, params, "the visitor's name stays one filename parameter")
	assert.Equal(t, "application/octet-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	content, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}

func TestAdminExportNeutralizesFieldNames(t *testing.T) {
	ts := mountTestServer(t)
	db := ts.DB.GetConnection()
	form := &forms.Form{Name: "Contact", Slug: "contact", AllowedOrigins: "example.com"}
	require.NoError(t, db.Create(form).Error)
	_, err := forms.CreateSubmission(slog.Default(), db, form, map[string]any{"=HYPERLINK(\"http://evil\")": "x"}, "test")
	require.NoError(t, err)
	get := signInGet(t, ts)

	resp := get("/admin/submissions/export.csv")

	require.Equal(t, 200, resp.StatusCode)
	rows, err := csv.NewReader(resp.Body).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"received", "form", "spam", `'=HYPERLINK("http://evil")`}, rows[0])
	assert.Equal(t, "x", rows[1][3])
}

func signInGet(t *testing.T, ts *cartridgetestsupport.TestServer) func(path string) *http.Response {
	t.Helper()
	seedAdmin(t, ts, "admin@example.com", "a-good-password")
	req := httptest.NewRequest("POST", "/admin/login", strings.NewReader("email=admin@example.com&password=a-good-password"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := ts.Server.Test(req)
	require.NoError(t, err)
	require.Equal(t, 302, resp.StatusCode)
	cookies := resp.Cookies()

	return func(path string) *http.Response {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		resp, err := ts.Server.Test(req)
		require.NoError(t, err)
		return resp
	}
}

// The installer checks /_health, and Chasen checks /up: both answer 200,
// to GET and HEAD, for any Host, with no login and no redirect.
func TestHealthPaths(t *testing.T) {
	ts := mountTestServer(t)

	for _, path := range []string{"/_health", "/up"} {
		for _, method := range []string{"GET", "HEAD"} {
			t.Run(method+" "+path, func(t *testing.T) {
				req := httptest.NewRequest(method, path, nil)
				req.Host = "formlander.internal"

				resp, err := ts.Server.Test(req)

				assert.NoError(t, err)
				assert.Equal(t, 200, resp.StatusCode)
			})
		}
	}
}
