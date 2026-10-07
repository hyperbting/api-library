package meta

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// https://developers.facebook.com/documentation/games_payments/webhooks
const webhookEventKey = "meta_webhook_event"

type WebhookLogger interface {
	SaveWebhookLog(ctx context.Context, provider string, payload []byte) error
}

type WebhookConfig struct {
	Payment *WebhookPaymentConfig `mapstructure:"payment"`
}

type WebhookPaymentConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	VerifyToken string `mapstructure:"verify_token"`
}

// PaymentEnabled is nil-safe: a missing payment section means disabled
func (c *WebhookConfig) PaymentEnabled() bool {
	return c != nil && c.Payment != nil && c.Payment.Enabled
}

// NewMetaPaymentWebhookMiddleware handle Receiving Updates
func NewMetaPaymentWebhookMiddleware[T any](appSecret string, cfg *WebhookConfig, logger WebhookLogger) fiber.Handler {
	return func(c fiber.Ctx) error {
		// POST only: Receiving Updates, reject if not
		if c.Method() != fiber.MethodPost {
			return c.Status(fiber.StatusMethodNotAllowed).SendString("Method Not Allowed")
		}

		// 驗證 Meta POST Webhook 簽名 (X-Hub-Signature-256)
		body := c.Body()
		signature := c.Get("X-Hub-Signature-256")
		isValid := verifyHMAC(body, signature, appSecret)

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
		} else {
			log.Printf("skipping webhook log: %s", string(body))
		}

		if err := parseThenSaveEvent[T](c); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Next()
	}
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

func parseThenSaveEvent[T any](c fiber.Ctx) error {
	var pi T
	if err := json.Unmarshal(c.Body(), &pi); err != nil {
		return ErrInvalidJSONPayload
	}
	c.Locals(webhookEventKey, &pi)
	return nil
}

func GetEvent[T any](c fiber.Ctx) (*T, error) {
	val := c.Locals(webhookEventKey)
	if val == nil {
		return nil, fiber.ErrBadRequest
	}

	if eventPtr, ok := val.(*T); ok {
		return eventPtr, nil
	}

	return nil, fiber.ErrInternalServerError
}
