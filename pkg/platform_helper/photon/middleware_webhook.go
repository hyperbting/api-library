package photon

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"

	"api-library/pkg/validator"
)

//https://doc.photonengine.com/pun/current/gameplay/web-extensions/webhooks

// WebhookPath represents standard Photon Webhook configured paths
type WebhookPath string

const (
	// PathCreate is called when creating a room or reloading room state
	PathCreate WebhookPath = "PathCreate"
	// PathClose is called when a room is removed from server memory
	PathClose WebhookPath = "PathClose"
	// PathJoin is called when a player joins an existing room in memory
	PathJoin WebhookPath = "PathJoin"
	// PathLeave is called when a player leaves a room
	PathLeave WebhookPath = "PathLeave"
	// PathEvent is called when a client raises an event with the HttpForward web flag
	PathEvent WebhookPath = "PathEvent"
	// PathGameProperties is called when room or player properties are updated
	PathGameProperties WebhookPath = "PathGameProperties"
	// PathBeforeJoin is called for authorization before a player joins (if configured)
	PathBeforeJoin WebhookPath = "PathBeforeJoin"
)

// String returns string representation
func (p WebhookPath) String() string {
	return string(p)
}

// PhotonWebhookBaseRequest represents common payload fields sent by Photon webhooks
type PhotonWebhookBaseRequest struct {
	AppId      string `json:"AppId"`
	AppVersion string `json:"AppVersion"`
	Region     string `json:"Region"`
	GameId     string `json:"GameId"`
	Type       string `json:"Type"`
	ActorNr    int    `json:"ActorNr,omitempty"`
	UserId     string `json:"UserId,omitempty"`
}

type webhookEventCtxKey struct{}

// webhookEventKey is the fiber Locals key under which the parsed webhook payload is stored.
var webhookEventKey = webhookEventCtxKey{}

// NewPhotonWebhookMiddleware parses the POST body into T, validates it, stores *T in
// the request context (read it back with GetEvent[T]) and passes down to the next handler.
// Photon expects HTTP 200 even on failure, so errors are reported via ResultCode.
func NewPhotonWebhookMiddleware[T any](logger WebhookLogger) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Method() != fiber.MethodPost {
			return c.Status(fiber.StatusMethodNotAllowed).SendString("Method Not Allowed")
		}

		var req T
		if err := c.Bind().JSON(&req); err != nil {
			return c.JSON(fiber.Map{"ResultCode": 1, "Message": ErrInvalidJSONPayload.Error()})
		}

		if err := validator.Struct(req); err != nil {
			return c.JSON(fiber.Map{"ResultCode": 1, "Message": ErrInvalidPayload.Error(), "Detail": err.Error()})
		}

		c.Locals(webhookEventKey, &req)

		if logger != nil {
			bgCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Context()), 5*time.Second)
			payload := append([]byte(nil), c.Body()...)
			go func() {
				defer cancel()
				_ = logger.SaveWebhookLog(bgCtx, "photon_webhook", payload)
			}()
		}

		return c.Next()
	}
}

// GetEvent returns the payload stored by NewPhotonWebhookMiddleware[T].
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
