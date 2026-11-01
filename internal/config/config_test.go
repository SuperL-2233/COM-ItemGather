package config

import "testing"

func TestDefaultUploadLimit(t *testing.T) {
	t.Setenv("MAX_UPLOAD_SIZE", "")
	t.Setenv("MAX_UPLOAD_MB", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxUploadBytes != 3*1024*1024 {
		t.Fatalf("default upload limit = %d, want %d", cfg.MaxUploadBytes, 3*1024*1024)
	}
}

func TestRetryAndBackgroundIntervals(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"AI_MAX_RETRIES", "4"}, {"AI_MAX_RETRIES", "-1"},
		{"AI_TIMEOUT", "0s"}, {"BACKUP_INTERVAL", "0s"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid retry count or background interval was accepted")
			}
		})
	}
	for _, value := range []string{"0", "3"} {
		t.Run("valid-retries="+value, func(t *testing.T) {
			t.Setenv("AI_MAX_RETRIES", value)
			if _, err := Load(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
