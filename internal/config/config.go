package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                string
	HTTPReadTimeout         time.Duration
	HTTPReadHeaderTimeout   time.Duration
	HTTPWriteTimeout        time.Duration
	HTTPIdleTimeout         time.Duration
	LogLevel                slog.Level
	ShutdownTimeout         time.Duration
	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration
	PositionCheckInterval   time.Duration
	PositionStaleAfter      time.Duration
	PositionWorkers         int
	PositionQueueSize       int
	PushHTTPURL             string
	PushRequestTimeout      time.Duration
	OTELServiceName         string
	OTELExporterEndpoint    string
	OTELExporterProtocol    string
}

func Load() (Config, error) {
	var cfg Config

	var err error
	if cfg.HTTPAddr, err = required("HTTP_ADDR"); err != nil {
		return Config{}, err
	}
	if cfg.HTTPReadTimeout, err = requiredDuration("HTTP_READ_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTPReadHeaderTimeout, err = requiredDuration("HTTP_READ_HEADER_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTPWriteTimeout, err = requiredDuration("HTTP_WRITE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTPIdleTimeout, err = requiredDuration("HTTP_IDLE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseURL, err = required("DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if cfg.LogLevel, err = logLevel(); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = requiredDuration("SHUTDOWN_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMaxConns, err = requiredInt32("DATABASE_MAX_CONNS"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMinConns, err = requiredInt32("DATABASE_MIN_CONNS"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseMaxConnLifetime, err = requiredDuration("DATABASE_MAX_CONN_LIFETIME"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseConnectTimeout, err = requiredDuration("DATABASE_CONNECT_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseQueryTimeout, err = requiredDuration("DATABASE_QUERY_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.PositionCheckInterval, err = requiredDuration("POSITION_CHECK_INTERVAL"); err != nil {
		return Config{}, err
	}
	if cfg.PositionStaleAfter, err = requiredDuration("POSITION_STALE_AFTER"); err != nil {
		return Config{}, err
	}
	if cfg.PositionWorkers, err = requiredPositiveInt("POSITION_WORKERS"); err != nil {
		return Config{}, err
	}
	if cfg.PositionQueueSize, err = requiredPositiveInt("POSITION_QUEUE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.PushHTTPURL, err = required("PUSH_HTTP_URL"); err != nil {
		return Config{}, err
	}
	if cfg.PushRequestTimeout, err = requiredDuration("PUSH_REQUEST_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.OTELServiceName, err = required("OTEL_SERVICE_NAME"); err != nil {
		return Config{}, err
	}
	if cfg.OTELExporterEndpoint, err = required("OTEL_EXPORTER_OTLP_ENDPOINT"); err != nil {
		return Config{}, err
	}
	if cfg.OTELExporterProtocol, err = required("OTEL_EXPORTER_OTLP_PROTOCOL"); err != nil {
		return Config{}, err
	}
	if cfg.OTELExporterProtocol != "http/protobuf" {
		return Config{}, fmt.Errorf("OTEL_EXPORTER_OTLP_PROTOCOL must be http/protobuf")
	}
	if cfg.DatabaseMaxConns <= 0 || cfg.DatabaseMinConns < 0 || cfg.DatabaseMinConns > cfg.DatabaseMaxConns {
		return Config{}, fmt.Errorf("DATABASE_MIN_CONNS must be non-negative and no greater than DATABASE_MAX_CONNS")
	}
	return cfg, nil
}

func required(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func requiredDuration(name string) (time.Duration, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}

func requiredInt32(name string) (int32, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return int32(parsed), nil
}

func requiredPositiveInt(name string) (int, error) {
	value, err := required(name)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func logLevel() (slog.Level, error) {
	value, err := required("LOG_LEVEL")
	if err != nil {
		return 0, err
	}
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}
}
