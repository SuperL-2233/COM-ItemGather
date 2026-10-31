package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv              string
	Addr                string
	PublicURL           string
	DatabasePath        string
	AttachmentDir       string
	WebDist             string
	AIBaseURL           string
	AIAPIKey            string
	AIModel             string
	AITimeout           time.Duration
	AIMaxRetries        int
	AIWorkers           int
	SessionTTL          time.Duration
	RememberTTL         time.Duration
	MaxUploadBytes      int64
	MaxAttachments      int
	MaxStorageBytes     int64
	BackupDir           string
	BackupInterval      time.Duration
	BackupKeep          int
	BackupRetentionDays int
	SecureCookie        bool
}

func Load() (Config, error) {
	publicURL := env("APP_PUBLIC_URL", env("PUBLIC_URL", "http://localhost:8088"))
	secureDefault := strings.HasPrefix(strings.ToLower(publicURL), "https://")
	sessionTTL := duration("SESSION_TTL", 7*24*time.Hour)
	if os.Getenv("SESSION_DEFAULT_DAYS") != "" {
		sessionTTL = time.Duration(integer("SESSION_DEFAULT_DAYS", 7)) * 24 * time.Hour
	}
	rememberTTL := duration("REMEMBER_TTL", 90*24*time.Hour)
	if os.Getenv("SESSION_REMEMBER_DAYS") != "" {
		rememberTTL = time.Duration(integer("SESSION_REMEMBER_DAYS", 90)) * 24 * time.Hour
	}
	maxUpload := int64(integer("MAX_UPLOAD_MB", 10)) * 1024 * 1024
	if raw := os.Getenv("MAX_UPLOAD_SIZE"); raw != "" {
		maxUpload = parseByteSize(raw, maxUpload)
	}
	cfg := Config{
		AppEnv: env("APP_ENV", "development"),
		Addr:   env("APP_LISTEN_ADDR", env("ADDR", "127.0.0.1:8088")), PublicURL: strings.TrimRight(publicURL, "/"),
		DatabasePath: env("DATABASE_PATH", "data/app.db"), AttachmentDir: env("ATTACHMENT_DIR", "data/attachments"),
		WebDist:   env("WEB_DIST", "web/dist"),
		AIBaseURL: strings.TrimRight(env("AI_BASE_URL", "https://api.deepseek.com/v1"), "/"), AIAPIKey: os.Getenv("AI_API_KEY"),
		AIModel: env("AI_MODEL", "deepseek-v4-flash"), AITimeout: duration("AI_TIMEOUT", 60*time.Second),
		AIMaxRetries: integer("AI_MAX_RETRIES", 3), AIWorkers: integer("AI_WORKER_CONCURRENCY", integer("AI_WORKERS", 1)),
		SessionTTL: sessionTTL, RememberTTL: rememberTTL,
		MaxUploadBytes: maxUpload, MaxAttachments: integer("MAX_ATTACHMENTS_PER_IDEA", integer("MAX_ATTACHMENTS", 10)),
		MaxStorageBytes: int64(integer("MAX_STORAGE_MB", 2048)) * 1024 * 1024,
		BackupDir:       env("BACKUP_DIR", "backups"), BackupInterval: duration("BACKUP_INTERVAL", 24*time.Hour), BackupKeep: integer("BACKUP_KEEP", 10), BackupRetentionDays: integer("BACKUP_RETENTION_DAYS", 30),
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
	if cfg.BackupKeep < 1 || cfg.BackupRetentionDays < 1 {
		return cfg, fmt.Errorf("backup retention must be positive")
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
func parseByteSize(raw string, fallback int64) int64 {
	units := []struct {
		suffix     string
		multiplier int64
	}{{"GIB", 1024 * 1024 * 1024}, {"GB", 1024 * 1024 * 1024}, {"MIB", 1024 * 1024}, {"MB", 1024 * 1024}, {"KIB", 1024}, {"KB", 1024}, {"B", 1}}
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	for _, unit := range units {
		if strings.HasSuffix(normalized, unit.suffix) {
			value, err := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(normalized, unit.suffix)), 10, 64)
			if err == nil && value > 0 && value <= 1<<30 {
				return value * unit.multiplier
			}
			return fallback
		}
	}
	if value, err := strconv.ParseInt(normalized, 10, 64); err == nil && value > 0 {
		return value
	}
	return fallback
}
