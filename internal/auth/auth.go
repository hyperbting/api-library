package auth

import "github.com/gofiber/fiber/v3"

// AuthProvider 定義統一的驗證介面
type AuthProvider interface {
	Middleware() fiber.Handler
}
