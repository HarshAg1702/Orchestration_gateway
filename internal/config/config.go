package config

import (
	"os"
	"strconv"
)

type Config struct {
	ServerPort string

	OllamaURL   string
	LlamaModel  string
	MistralModel string
	EmbedModel  string

	RedisAddr string
	RedisTTL  int // hours

	QdrantAddr       string
	QdrantCollection string
	QdrantThreshold  float64
	QdrantTTLDays    int

	PrometheusPort string
}

func Load() *Config {
	return &Config{
		ServerPort: getEnv("SERVER_PORT", "8080"),

		OllamaURL:    getEnv("OLLAMA_URL", "http://localhost:11434"),
		LlamaModel:   getEnv("LLAMA_MODEL", "llama3"),
		MistralModel: getEnv("MISTRAL_MODEL", "mistral"),
		EmbedModel:   getEnv("EMBED_MODEL", "nomic-embed-text"),

		RedisAddr: getEnv("REDIS_ADDR", "localhost:6379"),
		RedisTTL:  getEnvInt("REDIS_TTL_HOURS", 24),

		QdrantAddr:       getEnv("QDRANT_ADDR", "localhost:6334"),
		QdrantCollection: getEnv("QDRANT_COLLECTION", "prompt_cache"),
		QdrantThreshold:  getEnvFloat("QDRANT_THRESHOLD", 0.95),
		QdrantTTLDays:    getEnvInt("QDRANT_TTL_DAYS", 7),

		PrometheusPort: getEnv("PROMETHEUS_PORT", "9090"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
