package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
	Database DatabaseConfig `mapstructure:"database"`
	Server   ServerConfig   `mapstructure:"server"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Metrics  MetricsConfig  `mapstructure:"metrics"`
	Workers  int            `mapstructure:"workers"`
	GracefulShutdownTimeout time.Duration `mapstructure:"graceful_shutdown_timeout"`
}

type RabbitMQConfig struct {
	URL string `mapstructure:"url"`
}

type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type AuthConfig struct {
	JWTSecret         string        `mapstructure:"jwt_secret"`
	TokenExpiry       time.Duration `mapstructure:"token_expiry"`
	RefreshExpiry     time.Duration `mapstructure:"refresh_expiry"`
	RequireAuth       bool          `mapstructure:"require_auth"`
	AllowRegistration bool          `mapstructure:"allow_registration"`
}

type MetricsConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	Path         string `mapstructure:"path"`
	Port         int    `mapstructure:"port"`
	UpdateInterval time.Duration `mapstructure:"update_interval"`
}

func Load() (*Config, error) {
	v := viper.New()
	
	// Set default values
	v.SetDefault("server.host", "localhost")
	v.SetDefault("server.port", 8080)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("workers", 3)
	v.SetDefault("graceful_shutdown_timeout", "30s")
	
	// Auth defaults
	v.SetDefault("auth.jwt_secret", "your-256-bit-secret-key-for-development-only-change-in-production")
	v.SetDefault("auth.token_expiry", "24h")
	v.SetDefault("auth.refresh_expiry", "168h") // 7 days
	v.SetDefault("auth.require_auth", false)
	v.SetDefault("auth.allow_registration", false)
	
	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.path", "/metrics")
	v.SetDefault("metrics.port", 8080) // Same port as main server
	v.SetDefault("metrics.update_interval", "10s")
	
	// Read from config file
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}
	
	// Override with environment variables
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}
	
	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	
	return &cfg, nil
}

func validateConfig(cfg *Config) error {
	if cfg.RabbitMQ.URL == "" {
		return fmt.Errorf("rabbitmq.url is required")
	}
	
	if cfg.Database.URL == "" {
		return fmt.Errorf("database.url is required")
	}
	
	if cfg.Workers <= 0 {
		return fmt.Errorf("workers must be greater than 0")
	}
	
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	
	return nil
}