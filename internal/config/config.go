package config

import (
	"api-library/internal/infrastructure"
	"api-library/pkg/platform_helper/meta"
	"api-library/pkg/session"
	"encoding/json"
	"log"
	"strings"

	"github.com/spf13/viper"
)

type AppDetail struct {
	Env      string `mapstructure:"env"`
	AuthMode string `mapstructure:"auth"`
	Port     int    `mapstructure:"port"`
	Version  string `mapstructure:"version"`
	// RegisterEnabled toggles the email/password registration route. Absent means off.
	RegisterEnabled bool `mapstructure:"register_enabled"`
	// AllowDevSessionKey (env APP_ALLOW_DEV_SESSION_KEY) lets the email login run with the public development
	// session key from config.yaml. For local runs only (config.local.yaml); never set it on a deployed service.
	AllowDevSessionKey bool `mapstructure:"allow_dev_session_key"`
}

// DevSessionSecretKey is the signing key committed in config.yaml. It is public, so tokens signed with it are
// forgeable; a deployed service must override it with SESSION_SECRET_KEY.
const DevSessionSecretKey = "c2VjcmV0LWtleS1mb3ItZGV2ZWxvcG1lbnQtMzA4OQ"

// ClientConfig is the forced client update (see package clientver). Version names, e.g. "1.1".
type ClientConfig struct {
	// env CLIENT_MIN_VERSION: clients below this get 426 on /api/<v>/*. Empty = nobody is blocked.
	MinVersion string `mapstructure:"min_version"`
	// env CLIENT_LATEST_VERSION: shown by GET /version so a client can offer an optional "update available".
	LatestVersion string `mapstructure:"latest_version"`
}

type FirebaseConfig struct {
	Enabled             bool   `mapstructure:"enabled"`               // 是否啟用 Firebase 服務
	ProjectID           string `mapstructure:"project_id"`            // GCP/Firebase Project ID
	CredentialsFilePath string `mapstructure:"credentials_file_path"` // 本地開發使用的 Service Account JSON 路徑 (可選)
	UseFirestore        bool   `mapstructure:"use_firestore"`         // 是否初始化 Firestore Client
	UseStorage          bool   `mapstructure:"use_storage"`           // 是否初始化 Storage Client
	StorageBucket       string `mapstructure:"storage_bucket"`        // GCS Storage Bucket 名稱
	UseMessage          bool   `mapstructure:"use_message"`           // 是否初始化 Message Client
}

// AdminConfig controls the admin API and the embedded admin panel. Everything is off by default, so the public
// service can run the same image with both switched off. Two independent switches, three deployments:
//   - neither:               the game API only (the public service)
//   - Enabled:               the game API plus the /api/<v>/admin routes, no panel
//   - Enabled + PanelEnabled: the above plus the embedded panel under /admin
type AdminConfig struct {
	Enabled bool `mapstructure:"enabled"` // env ADMIN_ENABLED: the /api/<v>/admin routes
	// env ADMIN_PANEL_ENABLED: serve the embedded panel under /admin. Only takes effect with Enabled, the panel
	// has nothing to talk to without the admin routes.
	PanelEnabled bool `mapstructure:"panel_enabled"`
	// env ADMIN_REGISTER_EMAIL: POST /api/<v>/admin/register (creates a Firebase Auth email/password account,
	// no role) and a "create an account" link on the panel. Needs Enabled and LoginEmail. Off by default.
	RegisterEmail bool `mapstructure:"register_email"`
	// Sign-in methods the panel offers and the admin routes accept. Google is off by default.
	LoginGoogle bool `mapstructure:"login_google"`
	LoginEmail  bool `mapstructure:"login_email"`
	// env ADMIN_ACCEPT_CUSTOM_TOKEN: the admin routes also accept an ID token that came from a Firebase custom
	// token (sign_in_provider "custom"), for tools that sign one with the Firebase Admin service account (needs
	// IAM roles/iam.serviceAccountTokenCreator on it). The uid still needs the admin role. Not a panel login
	// method. Off by default.
	AcceptCustomToken bool `mapstructure:"accept_custom_token"`
	// Firebase web app config the panel loads at runtime (public values, not secrets).
	WebAPIKey     string `mapstructure:"web_api_key"`
	WebAuthDomain string `mapstructure:"web_auth_domain"`
	WebAppID      string `mapstructure:"web_app_id"`
}

type AppConfig struct {
	Admin      AdminConfig                 `mapstructure:"admin"`
	App        AppDetail                   `mapstructure:"app"`
	Client     ClientConfig                `mapstructure:"client"`
	Database  *infrastructure.DBConfig    `mapstructure:"database"`
	Redis      *infrastructure.RedisConfig `mapstructure:"redis"`
	Elastic    *infrastructure.ESConfig    `mapstructure:"elastic"`
	SessionJWT *session.Config             `mapstructure:"session"`
	Firebase   *FirebaseConfig             `mapstructure:"firebase"`

	Meta    *meta.PlatformConfig `mapstructure:"meta"`
	Webhook *WebhookConfig       `mapstructure:"webhook"`
}

// secretKeyParts: a config key containing one of these (case-insensitive) has its value masked in DebugPrint.
var secretKeyParts = []string{"secret", "pass", "token", "credential", "key"}

func isSecretKey(k string) bool {
	k = strings.ToLower(k)
	for _, p := range secretKeyParts {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}

// redact masks, in place, every non-empty string whose key looks like a secret. Empty values stay visible so a
// missing secret can still be told apart from a set one.
func redact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok && s != "" && isSecretKey(k) {
				t[k] = "***"
				continue
			}
			t[k] = redact(val)
		}
	case []any:
		for i := range t {
			t[i] = redact(t[i])
		}
	}
	return v
}

// DebugPrint logs the Config as indented JSON with secrets masked (the log ends up in Cloud Logging) and
// returns the same masked bytes.
func (c *AppConfig) DebugPrint() []byte {
	raw, _ := json.Marshal(c)
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		log.Printf("appCfg: not printable: %v", err)
		return nil
	}
	jsonBytes, _ := json.MarshalIndent(redact(generic), "", "  ")
	log.Printf("appCfg\n%s\n", jsonBytes)
	return jsonBytes
}

// SessionKeyUnsafe reports that the session section still holds the public development key and nothing opted in
// to it (app.allow_dev_session_key). Services that sign email-player tokens must not start that path then.
func (c *AppConfig) SessionKeyUnsafe() bool {
	return c != nil && c.SessionJWT != nil && c.SessionJWT.SecretKey == DevSessionSecretKey && !c.App.AllowDevSessionKey
}

// FirebaseEnabled is nil-safe: a missing firebase section means disabled
func (c *AppConfig) FirebaseEnabled() bool {
	return c != nil && c.Firebase != nil && c.Firebase.Enabled
}

type WebhookConfig struct {
	Meta *meta.WebhookConfig `mapstructure:"meta"`
}

// MetaPaymentEnabled is nil-safe: a missing webhook/meta/payment section means disabled
func (c *WebhookConfig) MetaPaymentEnabled() bool {
	return c != nil && c.Meta.PaymentEnabled()
}

func LoadConfig() (*AppConfig, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	v.SetConfigFile("config.yaml")
	_ = v.ReadInConfig()

	v.SetConfigFile("config.local.yaml")
	_ = v.MergeInConfig()

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
