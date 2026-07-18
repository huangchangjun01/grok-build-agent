package config

import (
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// InitLogger creates and configures a logrus.Logger based on the provided configuration.
func InitLogger(cfg *Config) *logrus.Logger {
	logger := logrus.New()

	level, err := logrus.ParseLevel(strings.ToLower(cfg.Logging.Level))
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	switch cfg.Logging.Format {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{})
	default:
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
	}

	var output io.Writer
	switch cfg.Logging.Output {
	case "file", "stderr":
		if cfg.Logging.Output == "file" {
			file, err := os.OpenFile(cfg.Logging.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				logger.Warnf("Failed to open log file %s, falling back to stdout: %v", cfg.Logging.FilePath, err)
				output = os.Stdout
			} else {
				output = file
			}
		} else {
			output = os.Stderr
		}
	default:
		output = os.Stdout
	}
	logger.SetOutput(output)

	return logger
}