package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type FileLogger struct {
	Logger  *slog.Logger
	Close   func() error
	Path    string
	Enabled bool
}

func Nop() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func NewFileLogger(dataDir string, debug bool) (FileLogger, error) {
	if !debug {
		return FileLogger{Logger: Nop(), Close: func() error { return nil }, Enabled: false}, nil
	}
	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return FileLogger{Logger: Nop(), Close: func() error { return nil }, Enabled: false}, err
	}
	path := filepath.Join(logDir, "engine.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return FileLogger{Logger: Nop(), Close: func() error { return nil }, Enabled: false}, err
	}
	handler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	})
	logger := slog.New(handler)
	return FileLogger{
		Logger:  logger,
		Close:   file.Close,
		Path:    path,
		Enabled: true,
	}, nil
}
