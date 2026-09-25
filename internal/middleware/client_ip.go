package middleware

import (
	"net"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// ClientIP returns the IP of the visitor, not of the reverse proxy.
//
// kamal-proxy runs on the private Docker network and appends the address of
// its peer to X-Forwarded-For. The last entry is therefore the only one a
// visitor cannot forge. Earlier entries come from the visitor and are ignored.
// The header is trusted only when the direct peer is a private or loopback
// address, so a visitor that reaches the app directly cannot spoof it.
func ClientIP(c *fiber.Ctx) string {
	peer := c.IP()
	ip := net.ParseIP(peer)
	if ip == nil || !(ip.IsPrivate() || ip.IsLoopback()) {
		return peer
	}

	forwarded := c.Get(fiber.HeaderXForwardedFor)
	if forwarded == "" {
		return peer
	}
	entries := strings.Split(forwarded, ",")
	last := strings.TrimSpace(entries[len(entries)-1])
	if net.ParseIP(last) == nil {
		return peer
	}
	return last
}
