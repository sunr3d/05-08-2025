package config

import (
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

func GetConfigFromEnv() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		log.Printf("Не удалось загрузить .env файл: \"%s\", продолжаем со значениями окружения по умолчанию\n", err.Error())
	}

	cfg := &Config{}

	if err := envconfig.Process("", cfg); err != nil {
		return nil, fmt.Errorf("envconfig.Process: %w", err)
	}

	return cfg, nil
}
