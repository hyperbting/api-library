package meta

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	VerifyAttestationTokenPath = "/platform_integrity/verify"
	BanWithAttestationPath     = "/platform_integrity/device_ban"
	DeviceBanStatusCheckPath   = "/platform_integrity/device_ban_status"
)

type VerifyAttestationTokenQueryDTO struct {
	AttestationToken string `json:"attestation_token"`
}

func (tq *VerifyAttestationTokenQueryDTO) formUrl(cfg *OCULUSPlatformConfig) (string, error) {
	//https://graph.oculus.com/platform_integrity/verify?token=<attestation_token>&access_token=<access_token>

	// Join the base URL and the path safely using url.JoinPath.
	urlOnly, err := url.JoinPath(cfg.PlatformServer, VerifyAttestationTokenPath)
	if err != nil {
		return "", err
	}

	// Create a new URL object.
	u, err := url.Parse(urlOnly)
	if err != nil {
		return "", err
	}

	// Create a url.Values map to hold query parameters. This automatically handles URL encoding.
	q := u.Query()
	q.Add("token", tq.AttestationToken)
	q.Add("access_token", cfg.FormAccessToken())

	// Encode the query parameters and assign them to the URL.
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func NewVerifyAttestationTokenQueryDTO(attestationToken string) VerifyAttestationTokenQueryDTO {
	return VerifyAttestationTokenQueryDTO{AttestationToken: attestationToken}
}

type VerifyAttestationTokenResponseDTO struct {
	Data []struct {
		Message *string `json:"message" enums:"success,invalid signature,token expired"`
		// Claims is a Base64-encoded JWS string containing DeviceId, Nonce, and Integrity status.
		ClaimsInBase64 *string `json:"claims" extensions:"x-format=base64"`
	} `json:"data"`

	Claim *AttestationClaimsDTO `json:"-"`
}

func (v *VerifyAttestationTokenResponseDTO) IsDeviceBanned() bool {

	if len(v.Data) == 0 || v.Data[0].Message == nil {
		// No data means we can't verify, treat as banned (or handle differently)
		return true
	}

	// Assuming we only care about the first element in `data`
	if !strings.EqualFold(*v.Data[0].Message, "success") {
		return true
	}

	// now check with claim
	if parseErr := v.ParseClaims(false); parseErr == nil {
		if v.Claim == nil {
			return true
		}
		return v.Claim.IsDeviceBanned()
	}

	return true
}

func (v *VerifyAttestationTokenResponseDTO) ParseClaims(force bool) error {
	if v.Claim != nil && !force {
		return nil
	}

	if len(v.Data) == 0 || v.Data[0].ClaimsInBase64 == nil {
		return ErrNotFound
	}

	raw := *v.Data[0].ClaimsInBase64
	// Base64URL decode
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return err
	}

	var claims AttestationClaimsDTO
	if err = json.Unmarshal(decoded, &claims); err != nil {
		return err
	}

	v.Claim = &claims
	return nil
}

type BanStatusRequestDTO struct {
	UniqueId string `json:"unique_id"`
	BanId    string `json:"ban_id"`
}

func (bsr *BanStatusRequestDTO) HasBanId() bool {
	// Returns true if BanId is not an empty string
	return bsr.BanId != ""
}

func (bsr *BanStatusRequestDTO) formUrl(cfg *OCULUSPlatformConfig) (string, error) {
	//https://graph.oculus.com/platform_integrity/device_ban_status?
	//unique_id =<unique_id>&
	//access_token=<access_token>

	// Join the base URL and the path safely using url.JoinPath.
	urlOnly, err := url.JoinPath(cfg.PlatformServer, DeviceBanStatusCheckPath)
	if err != nil {
		return "", err
	}

	// Create a new URL object.
	u, err := url.Parse(urlOnly)
	if err != nil {
		return "", err
	}

	// Create a url.Values map to hold query parameters. This automatically handles URL encoding.
	q := u.Query()
	q.Add("access_token", cfg.FormAccessToken())

	if bsr.HasBanId() {
		q.Add("ban_id", bsr.BanId)
	} else {
		q.Add("unique_id", bsr.UniqueId)
	}

	// Encode the query parameters and assign them to the URL.
	u.RawQuery = q.Encode()

	return u.String(), nil
}

type BanStatusResponseDTO struct {
	Data  []BanStatusDataDTO `json:"data,omitempty"`
	Error *OCApiErrorDTO     `json:"error,omitempty"`
}

func (r *BanStatusResponseDTO) Result() string {
	// 1. Handle API Errors
	if r.Error != nil {
		switch r.Error.ErrorSubcode {
		case 1614026:
			return "invalid unique_id: record not found or expired (IDs change every 30 days)"
		case 1614027:
			return "unauthorized: unique_id does not match the App ID in your access token"
		default:
			return r.Error.Message
		}
	}

	// 2. Handle Success (Check if data slice is not empty)
	if len(r.Data) > 0 {
		status := r.Data[0]
		if status.IsBanned {
			return fmt.Sprintf("Player is BANNED. Remaining: %d minutes", status.RemainingTimeInMinutes)
		}
		return "Player is not banned."
	}

	return "No status data available"
}

type BanStatusDataDTO struct {
	Message                string `json:"message"`
	IsBanned               bool   `json:"is_banned"`
	RemainingTimeInMinutes int    `json:"remaining_time_in_minute"`
}

type DeviceBanDTO struct {
	IsBanned         bool `json:"is_banned"`
	RemainingBanTime int  `json:"remaining_time_in_minute" validate:"gte=0,lte=52560000"`
}

func (b *DeviceBanDTO) IsCurrentlyBanned() string {
	// Converts bool to "true" or "false" string
	return strconv.FormatBool(b.IsBanned)
}

type DeviceBanRequestDTO struct {
	DeviceBanDTO
	UniqueId string `json:"unique_id"`
	BanId    string `json:"ban_id"`
}

func (dbr *DeviceBanRequestDTO) HasBanId() bool {
	// Returns true if BanId is not an empty string
	return dbr.BanId != ""
}

func (dbr *DeviceBanRequestDTO) formUrl(cfg *OCULUSPlatformConfig) string {
	//https://graph.oculus.com/platform_integrity/device_ban?
	//method=POST&
	//unique_id=<unique_id>&
	//is_banned=<is_banned>&
	//remaining_time_in_minute=<remaining_time_in_minute>&
	//access_token=<access_token>

	// Join the base URL and the path safely using url.JoinPath.
	urlOnly, err := url.JoinPath(cfg.PlatformServer, BanWithAttestationPath)
	if err != nil {
		panic(err) // Handle error appropriately
	}

	// Create a new URL object.
	u, err := url.Parse(urlOnly)
	if err != nil {
		panic(err) // Handle error appropriately
	}

	// Create a url.Values map to hold query parameters. This automatically handles URL encoding.
	q := u.Query()
	q.Add("method", "POST")
	q.Add("is_banned", dbr.IsCurrentlyBanned())
	q.Add("access_token", cfg.FormAccessToken())

	if dbr.HasBanId() {
		q.Add("ban_id", dbr.BanId)
	} else {
		q.Add("unique_id", dbr.UniqueId)
	}

	if dbr.IsBanned {
		q.Add("remaining_time_in_minute", fmt.Sprintf("%d", dbr.RemainingBanTime))
	}

	// Encode the query parameters and assign them to the URL.
	u.RawQuery = q.Encode()

	return u.String()
}

type OCApiErrorDTO struct {
	Message      string                 `json:"message"`
	Type         string                 `json:"type"`
	Code         int                    `json:"code"`
	ErrorSubcode int                    `json:"error_subcode"`
	FBTraceID    string                 `json:"fbtrace_id"`
	ErrorData    map[string]interface{} `json:"error_data"`
}

type BanResponseDTO struct {
	Message string         `json:"message,omitempty"`
	BanID   string         `json:"ban_id,omitempty"` // Empty string on reversal, populated on update
	Error   *OCApiErrorDTO `json:"error,omitempty"`
}

func (r *BanResponseDTO) Result() string {
	// 1. Priority: Handle Errors
	if r.Error != nil {
		switch r.Error.ErrorSubcode {
		case 1614033:
			return "ban record not found or already reversed"
		case 1614036:
			return "unauthorized: access token mismatch"
		default:
			return r.Error.Message
		}
	}

	// 2. Priority: Specific Success Scenarios
	if r.BanID != "" {
		return fmt.Sprintf("Ban active. New ID: %s", r.BanID)
	}

	// Since the Oculus docs say ban_id is "" on reversal:
	if r.Message == "Success" {
		return "Ban successfully reversed."
	}

	// 3. Fallback
	return "Unknown response state"
}

type AttestationClaimsDTO struct {
	RequestDetails struct {
		Exp       int64  `json:"exp"`
		Nonce     string `json:"nonce"`
		Timestamp int64  `json:"timestamp"`
	} `json:"request_details"`

	AppState struct {
		AppIntegrityState       string   `json:"app_integrity_state" enums:"NotEvaluated,StoreRecognized"`
		PackageCertSha256Digest []string `json:"package_cert_sha256_digest"`
		PackageId               string   `json:"package_id"`
		Version                 string   `json:"version"`
	} `json:"app_state"`

	DeviceState struct {
		DeviceIntegrityState string `json:"device_integrity_state" enums:"NotTrusted,Advanced"`
		UniqueId             string `json:"unique_id"`
	} `json:"device_state"`

	DeviceBan *DeviceBanDTO `json:"device_ban,omitempty"`
	BanId     string        `json:"ban_id,omitempty"`
}

func (c *AttestationClaimsDTO) IsDeviceBanned() bool {

	//Inside the token claims section the returned result will have a device_ban section only if the device is banned.
	//Otherwise the device_ban section will be omitted.
	if c.DeviceBan == nil {
		return false
	}

	return c.DeviceBan.IsBanned
}

type AttestationRecordDTO struct {
	PlatformID string               `json:"pfm_id"`
	Timestamp  time.Time            `json:"timestamp"`
	AppSource  string               `json:"app_source"`
	Claims     AttestationClaimsDTO `json:"claims"`
}
