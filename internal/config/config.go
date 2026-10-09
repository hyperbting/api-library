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

type AppConfig struct {
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
