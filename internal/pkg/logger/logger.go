package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"formlander/internal/config"
)

// Initialize sets up structured logging to stdout and a rotating file.
func Initialize(cfg *config.Config) (*slog.Logger, error) {
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case config.LogLevelDebug:
		level = slog.LevelDebug
	case config.LogLevelInfo:
		level = slog.LevelInfo
	case config.LogLevelWarn:
		level = slog.LevelWarn
	case config.LogLevelError:
		level = slog.LevelError
	}

	rotator := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogsDirectory, "formlander.log"),
		MaxSize:    cfg.LogsMaxSizeInMB,
		MaxBackups: cfg.LogsMaxBackups,
		MaxAge:     cfg.LogsMaxAgeInDays,
		Compress:   false,
	}

	// Multi-writer for both console and file
	multiWriter := io.MultiWriter(os.Stdout, rotator)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	handler := slog.NewJSONHandler(multiWriter, opts)
	return slog.New(handler), nil
}

// GormLogger adapts slog to gorm's logger.Interface.
type GormLogger struct {
	slogger *slog.Logger
	level   logger.LogLevel
	config  *gormLoggerConfig
}

type gormLoggerConfig struct {
	SlowThreshold             time.Duration
	IgnoreRecordNotFoundError bool
}

// NewGormLogger creates a gorm-compatible logger backed by slog.
func NewGormLogger(slogger *slog.Logger) logger.Interface {
	cfg := config.Get()

	gormLevel := logger.Warn
	switch cfg.LogLevel {
	case config.LogLevelDebug, config.LogLevelInfo:
		gormLevel = logger.Info
	case config.LogLevelWarn:
		gormLevel = logger.Warn
	case config.LogLevelError:
		gormLevel = logger.Error
	}

	return &GormLogger{
		slogger: slogger,
		level:   gormLevel,
		config: &gormLoggerConfig{
			SlowThreshold:             200 * time.Millisecond,
			IgnoreRecordNotFoundError: true,
		},
	}
}

func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	clone := *l
	clone.level = level
	return &clone
}

func (l *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Info {
		l.slogger.Info(fmt.Sprintf(msg, data...))
	}
}

func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Warn {
		l.slogger.Warn(fmt.Sprintf(msg, data...))
	}
}

func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Error {
		l.slogger.Error(fmt.Sprintf(msg, data...))
	}
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()
	sql = sanitizeSQL(sql)

	switch {
	case err != nil && (l.config.IgnoreRecordNotFoundError && errors.Is(err, gorm.ErrRecordNotFound)):
		return
	case err != nil:
		l.slogger.Error("gorm query failed",
			slog.Duration("elapsed", elapsed),
			slog.Int64("rows", rows),
			slog.String("sql", sql),
			slog.String("error", err.Error()),
		)
	case elapsed > l.config.SlowThreshold && l.level >= logger.Warn:
		l.slogger.Warn("gorm slow query",
			slog.Duration("elapsed", elapsed),
			slog.Int64("rows", rows),
			slog.String("sql", sql),
		)
	case l.level >= logger.Info:
		l.slogger.Debug("gorm query",
			slog.Duration("elapsed", elapsed),
			slog.Int64("rows", rows),
			slog.String("sql", sql),
		)
	}
}

func sanitizeSQL(sql string) string {
	sql = strings.TrimSpace(sql)
	sql = regexp.MustCompile(`\s+`).ReplaceAllString(sql, " ")
	if len(sql) > 500 {
		return sql[:500] + "..."
	}
	return sql
}
