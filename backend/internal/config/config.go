package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
	Environment string
}

func Load() Config {
	return Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://ditalk:ditalk@localhost:5432/ditalk?sslmode=disable"),
		Environment: getEnv("ENV", "development"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
