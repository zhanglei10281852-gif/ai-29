package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr           string
	DatabasePath       string
	BusinessTimezone   string
	ApprovalTaskTTL    time.Duration
	SessionTTL         time.Duration
	WorkerInterval     time.Duration
	WorkerSnapshotSize int
	ShutdownTimeout    time.Duration
	LogLevel           string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:         envOr("HTTP_ADDR", ":8080"),
		DatabasePath:     envOr("DATABASE_PATH", "./data/featuremesh.db"),
		BusinessTimezone: envOr("BUSINESS_TIMEZONE", "Asia/Shanghai"),
		LogLevel:         envOr("LOG_LEVEL", "info"),
	}
	var err error
	if cfg.ApprovalTaskTTL, err = durationEnv("APPROVAL_TTL", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.SessionTTL, err = durationEnv("SESSION_TTL", 12*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.WorkerInterval, err = durationEnv("WORKER_INTERVAL", 2*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = durationEnv("SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.WorkerSnapshotSize, err = intEnv("WORKER_BATCH_SIZE", 50); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var problems []error
	if strings.TrimSpace(c.HTTPAddr) == "" {
		problems = append(problems, errors.New("HTTP_ADDR is required"))
	}
	if strings.TrimSpace(c.DatabasePath) == "" {
		problems = append(problems, errors.New("DATABASE_PATH is required"))
	}
	if _, err := time.LoadLocation(c.BusinessTimezone); err != nil {
		problems = append(problems, fmt.Errorf("BUSINESS_TIMEZONE: %w", err))
	}
	if c.ApprovalTaskTTL < time.Minute || c.ApprovalTaskTTL > 24*time.Hour {
		problems = append(problems, errors.New("APPROVAL_TTL must be between one minute and one day"))
	}
	if c.SessionTTL < 15*time.Minute || c.SessionTTL > 30*24*time.Hour {
		problems = append(problems, errors.New("SESSION_TTL must be between fifteen minutes and thirty days"))
	}
	if c.WorkerInterval < 100*time.Millisecond || c.WorkerInterval > time.Minute {
		problems = append(problems, errors.New("WORKER_INTERVAL is outside supported range"))
	}
	if c.WorkerSnapshotSize < 1 || c.WorkerSnapshotSize > 1000 {
		problems = append(problems, errors.New("WORKER_BATCH_SIZE must be between 1 and 1000"))
	}
	if c.ShutdownTimeout < time.Second || c.ShutdownTimeout > time.Minute {
		problems = append(problems, errors.New("SHUTDOWN_TIMEOUT is outside supported range"))
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		problems = append(problems, errors.New("LOG_LEVEL must be debug, info, warn or error"))
	}
	return errors.Join(problems...)
}

func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := envOr(key, fallback.String())
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func intEnv(key string, fallback int) (int, error) {
	value := envOr(key, strconv.Itoa(fallback))
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}
