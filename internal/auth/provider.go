package auth

import "github.com/gofiber/fiber/v3"

// Firebase sign_in_provider values (the ID token's firebase.sign_in_provider claim).
const (
	ProviderPassword = "password"
	ProviderGoogle   = "google.com"
	// ProviderCustom is an ID token exchanged from a custom token minted with the Firebase Admin service account
	// (signJwt). Tools/publish_build.py uses it to post news; there is no panel login for it.
	ProviderCustom = "custom"
)

// RequireSignInProvider rejects (403) Firebase ID tokens that were not issued by one of the allowed
// sign-in methods, so a panel with Google login switched off also refuses Google tokens server-side.
// Mount it after the Firebase auth middleware. An empty allowed list rejects everyone.
func RequireSignInProvider(allowed ...string) fiber.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, p := range allowed {
		set[p] = struct{}{}
	}
	return func(c fiber.Ctx) error {
		user, ok := GetUser(c)
		if !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		if _, ok := set[user.SignInProvider]; !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "sign-in method not allowed"})
		}
		return c.Next()
	}
}
