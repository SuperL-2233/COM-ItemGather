package config

import "testing"

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
