# Package `auth`

`pkg/auth` provides authentication and session management services for applications within the repository. It manages JWT issuance/validation, token rotation, Redis-backed session tracking, and GoFiber (v2 and v3) authentication middlewares.

---

## Features

- **JWT Authentication**: Short-lived Access Tokens and long-lived Refresh Tokens with unique JTI claims (`jwt/v5`).
- **Session Tracking & Token Rotation**: Active refresh sessions tracked in Redis (`auth:session:<userID>:<jti>`). Rotating refresh tokens issues a new JTI and invalidates the previous session.
- **Multiple Login Methods**:
  - Email & Password registration/login via domain [`user.Service`](./service.go#L28).
  - Social / third-party identity registration and login (Google, Apple, Steam, etc.).
- **Fiber Middleware**:
  - [`AuthV2Middleware`](./middleware_fiber_v2.go#L9) for Fiber v2.
  - [`AuthV3Middleware`](./middleware_fiber_v3.go#L10) for Fiber v3.
  - Automatically extracts the `Bearer <token>` from the `Authorization` header, validates claims, and populates context locals (`user_id`, `roles`).

---

## Architecture Overview

```
                      +-----------------------------+
                      |   HTTP Request (Header)     |
                      +--------------+--------------+
                                     |
                                     v
                      +-----------------------------+
                      |  AuthV2 / AuthV3 Middleware |
                      +--------------+--------------+
                                     | ValidateAccessToken
                                     v
+------------------+         +---------------+         +-----------------------+
|  SessionRepo     |<------->|  auth.Service |<------->|  user.Service         |
|  (Redis storage) |         +-------+-------+         |  (Credential check)   |
+------------------+                 |                 +-----------------------+
                                     v
                             +---------------+
                             | TokenManager  |
                             | (JWT Sign/Ver)|
                             +---------------+
```

---

## Core Components

| File | Description |
| :--- | :--- |
| [`model.go`](./model.go) | Definitions for `Session`, `AccessClaims`, and `RefreshClaims`. |
| [`jwt.go`](./jwt.go) | `TokenManager` interface and implementation for signing and parsing JWT pairs. |
| [`repository.go`](./repository.go) | `SessionRepository` Redis interface and key format helpers (`auth:session:...`). |
| [`service.go`](./service.go) | Core `Service` handling register, login, and refresh token rotation. |
| [`middleware_fiber_v2.go`](./middleware_fiber_v2.go) | HTTP middleware for GoFiber v2. |
| [`middleware_fiber_v3.go`](./middleware_fiber_v3.go) | HTTP middleware for GoFiber v3. |
| [`error.go`](./error.go) | Common sentinel errors (e.g. `ErrInvalidCredentials`, `ErrTokenRevoked`). |

---

## Usage Example

### 1. Initialization

```go
package main

import (
	"time"

	"api-library/pkg/auth"
	"api-library/pkg/user"
	"github.com/redis/go-redis/v9"
)

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	sessionRepo := auth.NewSessionRepository(rdb)

	tokenManager := auth.NewTokenManager(auth.Config{
		SecretKey:  "your-secure-secret-key",
		Issuer:     "api-library",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
	})

	// Assuming userSvc is already initialized
	var userSvc user.Service
	authSvc := auth.NewService(tokenManager, sessionRepo, userSvc)
	_ = authSvc
}
```

### 2. Fiber v3 Middleware Integration

```go
app := fiber.New()

// Protected route group
api := app.Group("/api/v1", auth.AuthV3Middleware(tokenManager))

api.Get("/profile", func(c fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	roles := c.Locals("roles").([]string)

	return c.JSON(fiber.Map{
		"user_id": userID,
		"roles":   roles,
	})
})
```

---

## Redis Key Structure

- **Session**: `auth:session:<userID>:<jti>` (Stores JSON-serialized `Session`, default TTL: 24h)
- **Device**: `auth:device:<userID>`
- **Blacklist**: `auth:blacklist:<jti>`
