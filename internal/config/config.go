package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL  string
	RedisURL     string
	Port         string
	Env          string
	KAIAuthToken string
}

func Load() *Config {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := os.Getenv("COMULINE_ENV")
	if env == "" {
		env = "development"
	}

	return &Config{
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		RedisURL:     os.Getenv("REDIS_URL"),
		Port:         port,
		Env:          env,
		KAIAuthToken: os.Getenv("KAI_AUTH_TOKEN"),
	}
}
