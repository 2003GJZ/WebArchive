package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr             string
	BaseURL          string
	MySQLDSN         string
	MinIOEndpoint    string
	MinIOAccessKey   string
	MinIOSecretKey   string
	MinIOSecure      bool
	MinIOBucket      string
	HTTPTimeout      time.Duration
	LLMBaseURL       string
	LLMAPIKey        string
	LLMModel         string
	LLMTimeout       time.Duration
	LLMEnabled       bool
	AutoTagOnCapture bool
}

func Load() Config {
	return Config{
		Addr:             getenv("ADDR", ":8080"),
		BaseURL:          getenv("BASE_URL", "http://localhost:8080"),
		MySQLDSN:         getenv("MYSQL_DSN", "webarchive:webarchive@tcp(127.0.0.1:3306)/webarchive?charset=utf8mb4&parseTime=True&loc=Local"),
		MinIOEndpoint:    getenv("MINIO_ENDPOINT", "127.0.0.1:9000"),
		MinIOAccessKey:   getenv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:   getenv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOSecure:      getenvBool("MINIO_SECURE", false),
		MinIOBucket:      getenv("MINIO_BUCKET", "webarchive"),
		HTTPTimeout:      time.Duration(getenvInt("HTTP_TIMEOUT_SECONDS", 20)) * time.Second,
		LLMBaseURL:       getenv("LLM_BASE_URL", "https://api.openai.com/v1"),
		LLMAPIKey:        getenv("LLM_API_KEY", ""),
		LLMModel:         getenv("LLM_MODEL", ""),
		LLMTimeout:       time.Duration(getenvInt("LLM_TIMEOUT_SECONDS", 30)) * time.Second,
		LLMEnabled:       getenvBool("LLM_ENABLED", false),
		AutoTagOnCapture: getenvBool("AUTO_TAG_ON_CAPTURE", false),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		i, err := strconv.Atoi(v)
		if err == nil {
			return i
		}
	}
	return def
}
