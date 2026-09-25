package meta

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// https://developers.facebook.com/documentation/games_payments/webhooks

const WebhookEventKey = "meta_webhook_event"

type WebhookLogger interface {
	SaveWebhookLog(ctx context.Context, provider string, payload []byte) error
}

type WebhookConfig struct {
	VerifyToken string `mapstructure:"verify_token"`
	AppSecret   string `mapstructure:"app_secret"`
}

// NewMetaPaymentWebhookMiddleware handle Receiving Updates
func NewMetaPaymentWebhookMiddleware[T any](cfg *WebhookConfig, logger WebhookLogger) fiber.Handler {
	return func(c fiber.Ctx) error {
		// POST only: Receiving Updates, reject if not
		if c.Method() != fiber.MethodPost {
			return c.Status(fiber.StatusMethodNotAllowed).SendString("Method Not Allowed")
		}

		// 驗證 Meta POST Webhook 簽名 (X-Hub-Signature-256)
		body := c.Body()
		signature := c.Get("X-Hub-Signature-256")
		isValid := verifyHMAC(body, signature, cfg.AppSecret)

		if !isValid {
			// TODO: 簽名失敗亦可選擇性紀錄非法攻擊 Log
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid Signature"})
		}

		// 3. 呼叫 WebhookLogger 介面進行 Audit Log 寫入
		if logger != nil {
			bgCtx := context.WithoutCancel(c.Context())
			payloadCopy := bytes.Clone(body)

			go func(ctx context.Context, data []byte) {
				_ = logger.SaveWebhookLog(ctx, "meta_payment", data)
			}(bgCtx, payloadCopy)
		}

		// 4. 解析為強型別 DTO T
		var event T
		if err := json.Unmarshal(body, &event); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON Payload"})
		}

		// 5. 儲存解析後的物件至 Context
		c.Locals(WebhookEventKey, &event)
		return c.Next()
	}
}

// GetEvent 強型別 Loader 函式，供後續 Handler 安全取出 DTO
func GetEvent[T any](c fiber.Ctx) (*T, error) {
	val := c.Locals(WebhookEventKey)
	if val == nil {
		return nil, ErrEventNotFound
	}

	event, ok := val.(*T)
	if !ok {
		return nil, ErrInvalidEvent
	}

	return event, nil
}

func verifyHMAC(payload []byte, signatureHeader string, secret string) bool {
	if signatureHeader == "" || !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}

	actualSig := strings.TrimPrefix(signatureHeader, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(actualSig), []byte(expectedSig))
}
