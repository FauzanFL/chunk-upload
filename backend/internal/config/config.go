package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	App    AppConfig
	Server ServerConfig
	Redis  RedisConfig
	MinIO  MinIOConfig
	Upload UploadConfig
}

type AppConfig struct {
	Env            string
	AllowedOrigins []string
}

type ServerConfig struct {
	Port string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type UploadConfig struct {
	ChunkSizeBytes    int64
	MaxFileSizeBytes  int64
	UploadTTL         time.Duration
	AllowedMIMETypes  []string
}

// Load reads config from environment with sensible defaults.
func Load() *Config {
	return &Config{
		App: AppConfig{
			Env:            getEnv("APP_ENV", "development"),
			AllowedOrigins: getEnvSlice("ALLOWED_ORIGINS", []string{"http://localhost:5173"}),
		},
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		MinIO: MinIOConfig{
			Endpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin123"),
			Bucket:    getEnv("MINIO_BUCKET", "uploads"),
			UseSSL:    getEnvBool("MINIO_USE_SSL", false),
		},
		Upload: UploadConfig{
			ChunkSizeBytes:   getEnvInt64("CHUNK_SIZE_BYTES", 5*1024*1024), // 5 MB
			MaxFileSizeBytes: getEnvInt64("MAX_FILE_SIZE_BYTES", 10*1024*1024*1024), // 10 GB
			UploadTTL:        time.Duration(getEnvInt("UPLOAD_TTL_HOURS", 24)) * time.Hour,
			AllowedMIMETypes: getEnvSlice("ALLOWED_MIME_TYPES", []string{
				"video/mp4", "video/mpeg", "video/quicktime", "video/x-msvideo",
				"image/jpeg", "image/png", "image/gif", "image/webp",
				"application/pdf", "application/zip", "application/x-tar",
				"application/octet-stream",
				"audio/mpeg", "audio/wav", "audio/ogg",
			}),
		},
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvSlice(key string, defaultVal []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func getEnvInt64(key string, defaultVal int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return defaultVal
	}
	return n
}

func getEnvBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}
