package config

import (
	"api-library/internal/infrastructure"
	"api-library/pkg/session"
	"encoding/json"
	"log"
	"strings"

	"github.com/spf13/viper"
)

type AppDetail struct {
	Env     string `mapstructure:"env"`
	Port    int    `mapstructure:"port"`
	Version string `mapstructure:"version"`
}

type AppConfig struct {
	App        AppDetail                   `mapstructure:"app"`
	Database   *infrastructure.DBConfig    `mapstructure:"database"`
	Redis      *infrastructure.RedisConfig `mapstructure:"redis"`
	Elastic    *infrastructure.ESConfig    `mapstructure:"elastic"`
	SessionJWT *session.Config             `mapstructure:"session"`
}

// DebugPrint 格式化印出 Config
func (c *AppConfig) DebugPrint() []byte {
	jsonBytes, _ := json.MarshalIndent(c, "", "  ")
	log.Printf("appCfg\n%s\n", jsonBytes)
	return jsonBytes
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
