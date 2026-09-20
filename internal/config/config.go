package config

import (
	"api-library/internal/infrastructure"
	"fmt"

	"github.com/spf13/viper"
)

type AppConfig struct {
	Env      string `mapstructure:"APP_ENV"`
	Port     int    `mapstructure:"APP_PORT"`
	Database *infrastructure.DBConfig
	Redis    *infrastructure.RedisConfig
	Elastic  *infrastructure.ESConfig
}

func LoadConfig() (*AppConfig, error) {
	v := viper.New()

	// 1. 設定讀取 .env 檔
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".") // 在專案根目錄尋找 .env

	// 2. 允許直接讀取系統環境變數 (如 Docker / K8s 環境)
	v.AutomaticEnv()

	// 3. 嘗試讀取檔案 (若找不到 .env，如在容器內直接傳入 Env，則跳過)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}
