package meta

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// https://developers.facebook.com/documentation/games_payments/webhooks

type WebhookLogger interface {
	SaveWebhookLog(ctx context.Context, provider string, payload []byte) error
}

type WebhookConfig struct {
	VerifyToken string `mapstructure:"verify_token"`
	AppSecret   string `mapstructure:"app_secret"`
}

// NewMetaPaymentWebhookMiddleware handle Receiving Updates
func NewMetaPaymentWebhookMiddleware(cfg *WebhookConfig, logger WebhookLogger) fiber.Handler {
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
		} else {
			log.Printf("skipping webhook log: %s", string(body))
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
