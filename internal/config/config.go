package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr            string
	PublicURL       string
	DatabasePath    string
	AttachmentDir   string
	WebDist         string
	AIBaseURL       string
	AIAPIKey        string
	AIModel         string
	AITimeout       time.Duration
	AIMaxRetries    int
	AIWorkers       int
	SessionTTL      time.Duration
	RememberTTL     time.Duration
	MaxUploadBytes  int64
	MaxAttachments  int
	MaxStorageBytes int64
	BackupDir       string
	BackupInterval  time.Duration
	BackupKeep      int
	SecureCookie    bool
}

func Load() (Config, error) {
	publicURL := env("PUBLIC_URL", "http://localhost:8080")
	secureDefault := strings.HasPrefix(strings.ToLower(publicURL), "https://")
	cfg := Config{
		Addr: env("ADDR", "127.0.0.1:8080"), PublicURL: strings.TrimRight(publicURL, "/"),
		DatabasePath: env("DATABASE_PATH", "data/app.db"), AttachmentDir: env("ATTACHMENT_DIR", "data/attachments"),
		WebDist:   env("WEB_DIST", "web/dist"),
		AIBaseURL: strings.TrimRight(env("AI_BASE_URL", "https://api.deepseek.com/v1"), "/"), AIAPIKey: os.Getenv("AI_API_KEY"),
		AIModel: env("AI_MODEL", "deepseek-v4-flash"), AITimeout: duration("AI_TIMEOUT", 60*time.Second),
		AIMaxRetries: integer("AI_MAX_RETRIES", 3), AIWorkers: integer("AI_WORKERS", 1),
		SessionTTL: duration("SESSION_TTL", 7*24*time.Hour), RememberTTL: duration("REMEMBER_TTL", 90*24*time.Hour),
		MaxUploadBytes: int64(integer("MAX_UPLOAD_MB", 10)) * 1024 * 1024, MaxAttachments: integer("MAX_ATTACHMENTS", 10),
		MaxStorageBytes: int64(integer("MAX_STORAGE_MB", 2048)) * 1024 * 1024,
		BackupDir:       env("BACKUP_DIR", "backups"), BackupInterval: duration("BACKUP_INTERVAL", 24*time.Hour), BackupKeep: integer("BACKUP_KEEP", 7),
		SecureCookie: boolean("COOKIE_SECURE", secureDefault),
	}
	if cfg.AIMaxRetries < 0 || cfg.AIMaxRetries > 10 {
		return cfg, fmt.Errorf("AI_MAX_RETRIES must be between 0 and 10")
	}
	if cfg.AIWorkers < 1 || cfg.AIWorkers > 16 {
		return cfg, fmt.Errorf("AI_WORKERS must be between 1 and 16")
	}
	if cfg.MaxUploadBytes < 1 || cfg.MaxAttachments < 1 || cfg.MaxStorageBytes < cfg.MaxUploadBytes {
		return cfg, fmt.Errorf("invalid upload limits")
	}
	if cfg.SessionTTL <= 0 || cfg.RememberTTL < cfg.SessionTTL {
		return cfg, fmt.Errorf("invalid session durations")
	}
	if cfg.BackupKeep < 1 {
		return cfg, fmt.Errorf("BACKUP_KEEP must be positive")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func integer(key string, fallback int) int {
	value, err := strconv.Atoi(env(key, strconv.Itoa(fallback)))
	if err != nil {
		return fallback
	}
	return value
}
func boolean(key string, fallback bool) bool {
	value, err := strconv.ParseBool(env(key, strconv.FormatBool(fallback)))
	if err != nil {
		return fallback
	}
	return value
}
func duration(key string, fallback time.Duration) time.Duration {
	raw := env(key, fallback.String())
	if value, err := time.ParseDuration(raw); err == nil {
		return value
	}
	if value, err := strconv.Atoi(raw); err == nil {
		return time.Duration(value) * time.Second
	}
	return fallback
}
