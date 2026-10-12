package route

import (
	"net"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// clientKey identifies the caller of a rate-limited route.
//
// On Cloud Run the TCP peer is Google's front end, so c.IP() cannot tell clients apart. The front end
// appends the address it saw to X-Forwarded-For, so the LAST entry is the real client. The first entries are
// whatever the client sent and are never used (a client can put anything there). When the header is missing or
// its last entry is not an IP (local development, tests) the key falls back to c.IP().
//
// If the service is ever put behind another proxy (an HTTPS load balancer), that proxy's address becomes the
// last entry: take the one before it instead.
func clientKey(c fiber.Ctx) string {
	if ip := lastForwardedIP(c.Get(fiber.HeaderXForwardedFor)); ip != "" {
		return ip
	}
	return c.IP()
}

func lastForwardedIP(header string) string {
	if header == "" {
		return ""
	}
	last := header
	if i := strings.LastIndexByte(header, ','); i >= 0 {
		last = header[i+1:]
	}
	last = strings.TrimSpace(last)
	if net.ParseIP(last) == nil {
		return ""
	}
	return last
}
