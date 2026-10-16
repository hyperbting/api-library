// Package clientver is the forced client update: the server knows the lowest client version it still supports
// (env CLIENT_MIN_VERSION) and refuses older clients with 426 Upgrade Required. The version NAME decides, not the
// build code (user decision 2026-10-17): a breaking release needs a manual name bump in the Unity Player Settings.
package clientver

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// Header is what the client sends on every API call: Application.version, e.g. "1.0".
const Header = "X-Client-Version"

// Parse reads "1", "1.0" or "1.2.3" (an optional leading "v" and any "-suffix" are ignored) into numbers. ok is
// false for an empty or malformed string.
func Parse(s string) (parts []int, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+ "); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return nil, false
	}
	for _, f := range strings.Split(s, ".") {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}

// Less reports whether version a is lower than b, number by number; a missing part counts as 0 ("1.0" == "1.0.0").
// false when either side does not parse.
func Less(a, b string) bool {
	pa, okA := Parse(a)
	pb, okB := Parse(b)
	if !okA || !okB {
		return false
	}
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// Middleware answers 426 with {error, minClientVersion} when the request carries a client version below min. It
// fails open: no minimum configured, no header (builds from before this check do not send one) or a header that does
// not parse all pass.
func Middleware(min string) fiber.Handler {
	_, valid := Parse(min)
	return func(c fiber.Ctx) error {
		if valid {
			if v := c.Get(Header); v != "" && Less(v, min) {
				return c.Status(fiber.StatusUpgradeRequired).JSON(fiber.Map{
					"error":            "upgrade required",
					"minClientVersion": min,
				})
			}
		}
		return c.Next()
	}
}
