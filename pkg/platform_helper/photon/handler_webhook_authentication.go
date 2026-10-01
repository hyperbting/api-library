package photon

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
)

// https://doc.photonengine.com/pun/current/connection-and-authentication/authentication/custom-authentication

// If the received parameters are missing or are invalid the returned results should be
// { "ResultCode": 3, "Message": "Invalid parameters." }
// After finishing the validation the outcome should be returned as follow:
// Success: { "ResultCode": 1, "UserId": <userId> }
// Failure: { "ResultCode": 2, "Message": "Authentication failed. Wrong credentials." }

// CustomAuthResponse represents Photon Custom Authentication response schema
type CustomAuthResponse[T any] struct {
	ResultCode int    `json:"ResultCode"`
	UserId     string `json:"UserId,omitempty"`
	Nickname   string `json:"Nickname,omitempty"`
	Message    string `json:"Message,omitempty"`
	Data       *T     `json:"Data,omitempty"`
}

// Helpers
func AuthSuccessResponse[T any](userId string, nickname string, data *T) CustomAuthResponse[T] {
	return CustomAuthResponse[T]{
		ResultCode: 1,
		UserId:     userId,
		Nickname:   nickname,
		Data:       data,
	}
}

func AuthFailedResponse(msg string) CustomAuthResponse[any] {
	if msg == "" {
		msg = "Authentication failed. Wrong credentials."
	}
	return CustomAuthResponse[any]{
		ResultCode: 2,
		Message:    msg,
	}
}

func AuthInvalidParamResponse(msg string) CustomAuthResponse[any] {
	if msg == "" {
		msg = "Invalid parameters."
	}
	return CustomAuthResponse[any]{
		ResultCode: 3,
		Message:    msg,
	}
}

type WebhookLogger interface {
	SaveWebhookLog(ctx context.Context, provider string, payload []byte) error
}

type SessionValidator interface {
	// ValidateSession(ctx context.Context, token string) (userID string, err error)
	ValidateSessionByUserAndToken(ctx context.Context, userID string, token string) error
}

type WebhookConfig struct {
	Authentication *WebhookAuthenticationConfig `mapstructure:"authentication"`
}

type WebhookAuthenticationConfig struct {
	WebhookSecret    string `mapstructure:"webhook_secret" query:"webhook_secret"`
	WebhookSkipPhase string `mapstructure:"webhook_skip_phase" query:"webhook_skip_phase"`
}

func (cfg *WebhookAuthenticationConfig) Skip(phase string) bool {
	if cfg.WebhookSkipPhase == "" || phase == "" {
		return false
	}

	// if cfg.WebhookSkipPhase == "all" {
	// 	return true
	// }

	return subtle.ConstantTimeCompare([]byte(cfg.WebhookSkipPhase), []byte(phase)) == 1
}

func (cfg *WebhookAuthenticationConfig) SecretMatched(secret string) bool {
	if cfg.WebhookSecret == "" || secret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cfg.WebhookSecret), []byte(secret)) == 1
}

type PhotonAuthQuery struct {
	AppId      string `query:"app_id"`
	AppVersion string `query:"app_version"`
	Session    string `query:"session"`
	UserId     string `query:"user_id"`
	WebhookAuthenticationConfig
}

// NewPhotonAuthWebhookGetHandler handle Receiving Updates
func NewPhotonAuthWebhookGetHandler(cfg *WebhookConfig, sessionValidator SessionValidator, logger WebhookLogger) fiber.Handler {
	return func(c fiber.Ctx) error {

		if cfg == nil || cfg.Authentication == nil {
			return c.JSON(AuthInvalidParamResponse("Webhook config is not set"))
		}

		if sessionValidator == nil {
			return c.JSON(AuthInvalidParamResponse("Session validator is not set"))
		}

		// GET only: Receiving Updates, reject if not
		if c.Method() != fiber.MethodGet {
			//TODO: also log?
			return c.JSON(AuthInvalidParamResponse("Method Not Allowed")) //still 200!
		}

		var query PhotonAuthQuery
		if err := c.Bind().Query(&query); err != nil {
			//TODO: also log?
			return c.JSON(AuthInvalidParamResponse("")) //still 200!
		}

		if query.UserId == "" {
			//TODO: also log?
			return c.JSON(AuthInvalidParamResponse("Missing user_id"))
		}

		if cfg.Authentication.Skip(query.WebhookSkipPhase) {
			asyncLogAuthEvent(c.Context(), logger, "skipped", &query)
			return c.JSON(AuthSuccessResponse[any](query.UserId, "", nil))
		}

		if !cfg.Authentication.SecretMatched(query.WebhookSecret) {
			asyncLogAuthEvent(c.Context(), logger, "unmatched", &query)
			return c.JSON(AuthFailedResponse("")) //still 200!
		}

		if query.Session == "" {
			//TODO: also log?
			return c.JSON(AuthInvalidParamResponse("Missing session"))
		}

		if err := sessionValidator.ValidateSessionByUserAndToken(c.Context(), query.UserId, query.Session); err != nil {
			asyncLogAuthEvent(c.Context(), logger, "invalid", &query)
			return c.JSON(AuthFailedResponse("")) //still 200!
		}

		asyncLogAuthEvent(c.Context(), logger, "success", &query)
		return c.JSON(AuthSuccessResponse[any](query.UserId, "", nil))
	}
}

func asyncLogAuthEvent(ctx context.Context, logger WebhookLogger, status string, query *PhotonAuthQuery) {
	if logger == nil {
		log.Printf("[PhotonAuth] status=%s user_id=%s app_id=%s", status, query.UserId, query.AppId)
		return
	}

	payload, _ := json.Marshal(fiber.Map{
		"status":      status,
		"user_id":     query.UserId,
		"app_id":      query.AppId,
		"app_version": query.AppVersion,
	})

	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	go func(bg context.Context, dat []byte) {
		defer cancel()
		_ = logger.SaveWebhookLog(bg, "photon_auth", dat)
	}(bgCtx, payload)
}
