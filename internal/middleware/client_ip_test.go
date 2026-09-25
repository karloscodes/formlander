package middleware

import (
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startClientIPServer serves ClientIP on a loopback listener, so the direct
// peer is 127.0.0.1, as it is for kamal-proxy on the Docker network.
func startClientIPServer(t *testing.T) string {
	t.Helper()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString(ClientIP(c))
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })
	return "http://" + ln.Addr().String()
}

func getClientIP(t *testing.T, url, forwardedFor string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func TestClientIP(t *testing.T) {
	url := startClientIPServer(t)

	t.Run("uses the entry the proxy appended", func(t *testing.T) {
		ip := getClientIP(t, url, "203.0.113.7")

		assert.Equal(t, "203.0.113.7", ip)
	})

	t.Run("ignores entries the visitor forged", func(t *testing.T) {
		ip := getClientIP(t, url, "1.2.3.4, 5.6.7.8, 203.0.113.7")

		assert.Equal(t, "203.0.113.7", ip)
	})

	t.Run("falls back to the peer without the header", func(t *testing.T) {
		ip := getClientIP(t, url, "")

		assert.Equal(t, "127.0.0.1", ip)
	})

	t.Run("falls back to the peer when the last entry is not an IP", func(t *testing.T) {
		ip := getClientIP(t, url, "203.0.113.7, garbage")

		assert.Equal(t, "127.0.0.1", ip)
	})
}
