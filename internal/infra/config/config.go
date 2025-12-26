package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	Telegram TelegramConfig `yaml:"telegram"`
	Pushover PushoverConfig `yaml:"pushover"`
	Watcher  WatcherConfig  `yaml:"watcher"`
	Notifier NotifierConfig `yaml:"notifier"`
}

type ServerConfig struct {
	Environment string `yaml:"environment"` // dev, prod
	LogLevel    string `yaml:"log_level"`   // debug, info, warn, error
}

type DatabaseConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	User            string        `yaml:"user"`
	Password        string        `yaml:"password"`
	Database        string        `yaml:"database"`
	SSLMode         string        `yaml:"ssl_mode"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type TelegramConfig struct {
	BotToken      string        `yaml:"bot_token"`
	RateLimit     int           `yaml:"rate_limit"` // messages per minute
	RetryAttempts int           `yaml:"retry_attempts"`
	RetryDelay    time.Duration `yaml:"retry_delay"`
}

type PushoverConfig struct {
	APIToken      string        `yaml:"api_token"`
	RateLimit     int           `yaml:"rate_limit"` // messages per minute
	RetryAttempts int           `yaml:"retry_attempts"`
	RetryDelay    time.Duration `yaml:"retry_delay"`
}

type WatcherConfig struct {
	SubscribeDebounce time.Duration `yaml:"subscribe_debounce"` // delay before unsubscribe
	ReconnectDelay    time.Duration `yaml:"reconnect_delay"`
	PingInterval      time.Duration `yaml:"ping_interval"`
}

type NotifierConfig struct {
	Workers        int           `yaml:"workers"`          // number of worker goroutines
	PollInterval   time.Duration `yaml:"poll_interval"`    // how often to check for jobs
	MaxRetries     int           `yaml:"max_retries"`      // max retry attempts per job
	StuckJobWindow time.Duration `yaml:"stuck_job_window"` // time before job considered stuck
}

// Load reads and parses the config file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Set defaults
	cfg.setDefaults()

	// Validate
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) setDefaults() {
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 25
	}
	if c.Database.MaxIdleConns == 0 {
		c.Database.MaxIdleConns = 5
	}
	if c.Database.ConnMaxLifetime == 0 {
		c.Database.ConnMaxLifetime = 5 * time.Minute
	}
	if c.Watcher.SubscribeDebounce == 0 {
		c.Watcher.SubscribeDebounce = 30 * time.Second
	}
	if c.Watcher.ReconnectDelay == 0 {
		c.Watcher.ReconnectDelay = 5 * time.Second
	}
	if c.Watcher.PingInterval == 0 {
		c.Watcher.PingInterval = 30 * time.Second
	}
	if c.Notifier.Workers == 0 {
		c.Notifier.Workers = 3
	}
	if c.Notifier.PollInterval == 0 {
		c.Notifier.PollInterval = 1 * time.Second
	}
	if c.Notifier.MaxRetries == 0 {
		c.Notifier.MaxRetries = 3
	}
	if c.Notifier.StuckJobWindow == 0 {
		c.Notifier.StuckJobWindow = 5 * time.Minute
	}
}

func (c *Config) validate() error {
	if c.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}
	if c.Database.User == "" {
		return fmt.Errorf("database.user is required")
	}
	if c.Database.Database == "" {
		return fmt.Errorf("database.database is required")
	}
	if c.Telegram.BotToken == "" {
		return fmt.Errorf("telegram.bot_token is required")
	}
	if c.Pushover.APIToken == "" {
		return fmt.Errorf("pushover.api_token is required")
	}
	return nil
}

// DSN returns PostgreSQL connection string
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}
