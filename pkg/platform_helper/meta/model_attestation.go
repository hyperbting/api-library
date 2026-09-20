package meta

import "time"

type AttestationClaims struct {
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

func (c *AttestationClaims) IsDeviceBanned() bool {

	//Inside the token claims section the returned result will have a device_ban section only if the device is banned.
	//Otherwise the device_ban section will be omitted.
	if c.DeviceBan == nil {
		return false
	}

	return c.DeviceBan.IsBanned
}

type ESAttestationRecord struct {
	AppScopedID string            `json:"app_scoped_id"`
	Timestamp   time.Time         `json:"timestamp"`
	AppSource   string            `json:"app_source"`
	Claims      AttestationClaims `json:"claims"`
}

type ActionType string

const (
	ActionBan   ActionType = "BAN"
	ActionUnban ActionType = "UNBAN"
)

type ESDeviceBanRecord struct {
	AppScopedID      string     `json:"app_scoped_id"`
	Action           ActionType `json:"action"`
	UniqueId         string     `json:"unique_id"`
	BanId            string     `json:"ban_id"`
	RemainingBanTime int        `json:"remaining_time_in_minute" validate:"gte=0,lte=52560000"`
	Reason           string     `json:"ban_reason"`
	ExecutedAt       time.Time  `json:"executed_at"`
	Timestamp        time.Time  `json:"timestamp"`
}
