package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Config is Receiptd's fully-decoded YAML configuration, matching the
// schema documented in docs/ARCHITECTURE.md §7.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Auth      AuthConfig      `yaml:"auth"`
	Logging   LoggingConfig   `yaml:"logging"`
	Assets    AssetsConfig    `yaml:"assets"`
	Queue     QueueConfig     `yaml:"queue"`
	Printers  []PrinterConfig `yaml:"printers"`
	Providers ProvidersConfig `yaml:"providers"`
	Web       WebConfig       `yaml:"web"`
}

// ServerConfig is the server: section.
type ServerConfig struct {
	Address string `yaml:"address"`
}

// AuthConfig is the auth: section. Resolving TokenFile or the
// RECEIPTD_AUTH_TOKEN env override into an actual credential is the auth
// package's job, not config's — this only carries the settings.
type AuthConfig struct {
	Enabled   bool   `yaml:"enabled"`
	TokenFile string `yaml:"token_file"`
}

// LoggingConfig is the logging: section.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// AssetsConfig is the assets: section.
type AssetsConfig struct {
	Path string `yaml:"path"`
}

// QueueConfig is the queue: section.
type QueueConfig struct {
	Store        string
	Path         string
	MaxAttempts  int
	RetryBackoff time.Duration
}

// UnmarshalYAML decodes a QueueConfig from its flat YAML block, parsing
// RetryBackoff from a Go duration string (e.g. "5s") since yaml.v3 has no
// built-in support for time.Duration.
func (q *QueueConfig) UnmarshalYAML(value *yaml.Node) error {
	var raw struct {
		Store        string `yaml:"store"`
		Path         string `yaml:"path"`
		MaxAttempts  int    `yaml:"max_attempts"`
		RetryBackoff string `yaml:"retry_backoff"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}

	q.Store = raw.Store
	q.Path = raw.Path
	q.MaxAttempts = raw.MaxAttempts
	if raw.RetryBackoff != "" {
		d, err := time.ParseDuration(raw.RetryBackoff)
		if err != nil {
			return fmt.Errorf("queue.retry_backoff: %w", err)
		}
		q.RetryBackoff = d
	}
	return nil
}

// ProvidersConfig is the providers: section.
type ProvidersConfig struct {
	Weather WeatherProviderConfig `yaml:"weather"`
}

// WeatherProviderConfig is the providers.weather: section.
type WeatherProviderConfig struct {
	Driver    string `yaml:"driver"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// WebConfig is the web: section.
type WebConfig struct {
	Enabled bool `yaml:"enabled"`
}

// Load reads and parses the YAML configuration file at path, then
// validates it against Config.Validate. A missing file is reported as
// apperr.KindNotFound; any other read failure (e.g. permission denied) as
// apperr.KindPermanent; malformed YAML or a schema/validation failure as
// apperr.KindValidation.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, apperr.Wrap(apperr.KindNotFound, "config.Load", err)
		}
		return nil, apperr.Wrap(apperr.KindPermanent, "config.Load", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, apperr.Wrap(apperr.KindValidation, "config.Load", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}
