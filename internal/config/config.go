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
	Database   *infrastructure.DBConfig    `mapstructure:"database"`
	Redis      *infrastructure.RedisConfig `mapstructure:"redis"`
	Elastic    *infrastructure.ESConfig    `mapstructure:"elastic"`
	SessionJWT *session.Config             `mapstructure:"session"`
	Firebase   *FirebaseConfig             `mapstructure:"firebase"`

	Meta    *meta.PlatformConfig `mapstructure:"meta"`
	Webhook *WebhookConfig       `mapstructure:"webhook"`
}

// DebugPrint 格式化印出 Config
func (c *AppConfig) DebugPrint() []byte {
	jsonBytes, _ := json.MarshalIndent(c, "", "  ")
	log.Printf("appCfg\n%s\n", jsonBytes)
	return jsonBytes
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
