package config

import (
	"api-library/internal/database"
	"os"
)

type AppConfig struct {
	Database database.DBConfig
}

func Load() (*AppConfig, error) {
	return &AppConfig{
		Database: database.DBConfig{
			Host:     os.Getenv("DB_HOST"),
			Port:     5432,
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASSWORD"),
			DBName:   os.Getenv("DB_NAME"),
		},
	}, nil
}
