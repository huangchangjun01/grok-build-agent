package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	LLM       LLMConfig       `mapstructure:"llm"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Workspace WorkspaceConfig `mapstructure:"workspace"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Agent     AgentConfig     `mapstructure:"agent"`
	Tools     ToolsConfig     `mapstructure:"tools"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

// LLMConfig holds LLM provider configuration.
type LLMConfig struct {
	Provider              string        `mapstructure:"provider"`
	BaseURL               string        `mapstructure:"base_url"`
	APIKey                string        `mapstructure:"api_key"`
	DefaultModel          string        `mapstructure:"default_model"`
	WebSearchModel        string        `mapstructure:"web_search_model"`
	ImageDescriptionModel string        `mapstructure:"image_description_model"`
	SessionSummaryModel   string        `mapstructure:"session_summary_model"`
	Models                []ModelConfig `mapstructure:"models"`
}

// ModelConfig holds individual model configuration.
type ModelConfig struct {
	Model         string  `mapstructure:"model"`
	Name          string  `mapstructure:"name"`
	Description   string  `mapstructure:"description"`
	ContextWindow int     `mapstructure:"context_window"`
	Temperature   float64 `mapstructure:"temperature"`
	TopP          float64 `mapstructure:"top_p"`
	MaxTokens     int     `mapstructure:"max_tokens"`
}

// DatabaseConfig holds database configuration.
type DatabaseConfig struct {
	Driver          string        `mapstructure:"driver"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// StorageConfig holds storage paths configuration.
type StorageConfig struct {
	SessionsDir string `mapstructure:"sessions_dir"`
	JSONLDir    string `mapstructure:"jsonl_dir"`
}

// WorkspaceConfig holds workspace configuration.
type WorkspaceConfig struct {
	DefaultWorkDir  string   `mapstructure:"default_work_dir"`
	MaxFileSize     int64    `mapstructure:"max_file_size"`
	AllowedCommands []string `mapstructure:"allowed_commands"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level    string `mapstructure:"level"`
	Format   string `mapstructure:"format"`
	Output   string `mapstructure:"output"`
	FilePath string `mapstructure:"file_path"`
}

// AgentConfig holds agent behavior configuration.
type AgentConfig struct {
	MaxTurns            int     `mapstructure:"max_turns"`
	ContextWindow       int     `mapstructure:"context_window"`
	CompactionThreshold float64 `mapstructure:"compaction_threshold"`
	EnablePlanMode      bool    `mapstructure:"enable_plan_mode"`
	EnableGoalSystem    bool    `mapstructure:"enable_goal_system"`
	EnableSubagent      bool    `mapstructure:"enable_subagent"`
	EnableMCP           bool    `mapstructure:"enable_mcp"`
}

// ToolsConfig holds tools configuration.
type ToolsConfig struct {
	Permissions map[string]string `mapstructure:"permissions"`
}

// Load reads configuration from config.yaml and returns a Config.
func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// Also look for the config in the directory where the binary is located
	if exePath, err := os.Executable(); err == nil {
		viper.AddConfigPath(exePath)
	}

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}