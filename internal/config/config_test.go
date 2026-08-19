package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	for _, key := range []string{"HTTP_ADDR", "DATABASE_PATH", "BUSINESS_TIMEZONE", "APPROVAL_TTL", "SESSION_TTL", "WORKER_INTERVAL", "WORKER_BATCH_SIZE", "SHUTDOWN_TIMEOUT", "LOG_LEVEL"} {
		t.Setenv(key, "")
	}
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("DATABASE_PATH", "./data/test.db")
	t.Setenv("BUSINESS_TIMEZONE", "Asia/Shanghai")
	t.Setenv("APPROVAL_TTL", "30m")
	t.Setenv("SESSION_TTL", "12h")
	t.Setenv("WORKER_INTERVAL", "2s")
	t.Setenv("WORKER_BATCH_SIZE", "50")
	t.Setenv("SHUTDOWN_TIMEOUT", "10s")
	t.Setenv("LOG_LEVEL", "info")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.DatabasePath != "./data/test.db" {
		t.Fatalf("config = %+v", cfg)
	}
	if cfg.ApprovalTaskTTL != 30*time.Minute || cfg.SessionTTL != 12*time.Hour {
		t.Fatalf("durations = %+v", cfg)
	}
	if cfg.WorkerSnapshotSize != 50 || cfg.LogLevel != "info" {
		t.Fatalf("worker config = %+v", cfg)
	}
}

func TestLoadRejectsMalformedEnvironment(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"approval task duration", "APPROVAL_TTL", "soon"},
		{"session duration", "SESSION_TTL", "tomorrow"},
		{"worker duration", "WORKER_INTERVAL", "fast"},
		{"batch integer", "WORKER_BATCH_SIZE", "many"},
		{"shutdown duration", "SHUTDOWN_TIMEOUT", "later"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidEnvironment(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("Load error = %v", err)
			}
		})
	}
}

func TestValidateAggregatesProblems(t *testing.T) {
	cfg := Config{HTTPAddr: "", DatabasePath: "", BusinessTimezone: "Missing/Timezone", ApprovalTaskTTL: time.Second, SessionTTL: time.Minute, WorkerInterval: time.Millisecond, WorkerSnapshotSize: 0, ShutdownTimeout: time.Millisecond, LogLevel: "verbose"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("invalid config passed")
	}
	for _, fragment := range []string{"HTTP_ADDR", "DATABASE_PATH", "BUSINESS_TIMEZONE", "APPROVAL_TTL", "SESSION_TTL", "WORKER_INTERVAL", "WORKER_BATCH_SIZE", "SHUTDOWN_TIMEOUT", "LOG_LEVEL"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q missing %s", err, fragment)
		}
	}
}

func TestValidateBoundaryValues(t *testing.T) {
	base := Config{HTTPAddr: ":8080", DatabasePath: "db", BusinessTimezone: "UTC", ApprovalTaskTTL: time.Minute, SessionTTL: 15 * time.Minute, WorkerInterval: 100 * time.Millisecond, WorkerSnapshotSize: 1, ShutdownTimeout: time.Second, LogLevel: "debug"}
	if err := base.Validate(); err != nil {
		t.Fatalf("minimum boundaries: %v", err)
	}
	base.ApprovalTaskTTL = 24 * time.Hour
	base.SessionTTL = 30 * 24 * time.Hour
	base.WorkerInterval = time.Minute
	base.WorkerSnapshotSize = 1000
	base.ShutdownTimeout = time.Minute
	base.LogLevel = "error"
	if err := base.Validate(); err != nil {
		t.Fatalf("maximum boundaries: %v", err)
	}
}

func TestAcceptedLogLevels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		cfg := Config{HTTPAddr: ":8080", DatabasePath: "db", BusinessTimezone: "UTC", ApprovalTaskTTL: time.Minute, SessionTTL: time.Hour, WorkerInterval: time.Second, WorkerSnapshotSize: 10, ShutdownTimeout: time.Second, LogLevel: level}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("level %s: %v", level, err)
		}
	}
}

func setValidEnvironment(t *testing.T) {
	t.Helper()
	values := map[string]string{"HTTP_ADDR": ":8080", "DATABASE_PATH": "db", "BUSINESS_TIMEZONE": "UTC", "APPROVAL_TTL": "30m", "SESSION_TTL": "12h", "WORKER_INTERVAL": "1s", "WORKER_BATCH_SIZE": "10", "SHUTDOWN_TIMEOUT": "5s", "LOG_LEVEL": "info"}
	for key, value := range values {
		t.Setenv(key, value)
	}
}
