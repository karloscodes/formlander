package logger

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"formlander/internal/config"
)

// Initialize sets up structured logging to stdout and a rotating file.
func Initialize(cfg *config.Config) (*zap.Logger, error) {
	consoleEncoder := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	})

	fileEncoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	})

	level := zapcore.InfoLevel
	switch cfg.LogLevel {
	case config.LogLevelDebug:
		level = zapcore.DebugLevel
	case config.LogLevelInfo:
		level = zapcore.InfoLevel
	case config.LogLevelWarn:
		level = zapcore.WarnLevel
	case config.LogLevelError:
		level = zapcore.ErrorLevel
	}

	rotator := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogsDirectory, "formlander.log"),
		MaxSize:    cfg.LogsMaxSizeInMB,
		MaxBackups: cfg.LogsMaxBackups,
		MaxAge:     cfg.LogsMaxAgeInDays,
		Compress:   false,
	}

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), level),
		zapcore.NewCore(fileEncoder, zapcore.AddSync(rotator), level),
	)

	return zap.New(core, zap.AddCaller()), nil
}

// GormLogger adapts zap to gorm's logger.Interface.
type GormLogger struct {
	zapLogger *zap.Logger
	level     logger.LogLevel
	config    *gormLoggerConfig
}

type gormLoggerConfig struct {
	SlowThreshold             time.Duration
	IgnoreRecordNotFoundError bool
}

// NewGormLogger creates a gorm-compatible logger backed by zap.
func NewGormLogger(zapLogger *zap.Logger) logger.Interface {
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
		zapLogger: zapLogger,
		level:     gormLevel,
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
		l.zapLogger.Sugar().Infof(msg, data...)
	}
}

func (l *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Warn {
		l.zapLogger.Sugar().Warnf(msg, data...)
	}
}

func (l *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Error {
		l.zapLogger.Sugar().Errorf(msg, data...)
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
		l.zapLogger.Error("gorm query failed",
			zap.Duration("elapsed", elapsed),
			zap.Int64("rows", rows),
			zap.String("sql", sql),
			zap.Error(err),
		)
	case elapsed > l.config.SlowThreshold && l.level >= logger.Warn:
		l.zapLogger.Warn("gorm slow query",
			zap.Duration("elapsed", elapsed),
			zap.Int64("rows", rows),
			zap.String("sql", sql),
		)
	case l.level >= logger.Info:
		l.zapLogger.Debug("gorm query",
			zap.Duration("elapsed", elapsed),
			zap.Int64("rows", rows),
			zap.String("sql", sql),
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
