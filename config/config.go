package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config represents the application configuration
type Config struct {
	DefaultBackend BackendConfig            `yaml:"default_backend"`
	VirtualHosts   map[string]BackendConfig `yaml:"virtualhosts"`
	Frontend       FrontendConfig           `yaml:"frontend"`
	Cache          CacheConfig              `yaml:"cache"`
	Logging        LoggingConfig            `yaml:"logging"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug,info,warn,error
	Format string `yaml:"format"` // json or text
}

// BackendConfig contains backend-specific configuration
type BackendConfig struct {
	Target  string        `yaml:"target"`
	Timeout time.Duration `yaml:"timeout"`
}

// ParseTarget parses the target baseUrl into scheme, host and port
func (bc *BackendConfig) ParseTarget() (string, string, int, error) {
	u, err := url.Parse(bc.Target)
	if err != nil {
		return "", "", 0, fmt.Errorf("url.Parse(%q): %w", bc.Target, err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		port = 80
	}
	return u.Scheme, u.Hostname(), port, nil

}

// FrontendConfig contains frontend-specific configuration
type FrontendConfig struct {
	BaseURL     string `yaml:"base_url"`
	MetricsPort int    `yaml:"metricsport"`
	Cert        string `yaml:"cert"`
	Key         string `yaml:"key"`
}

// GetListenAddr returns the formatted listen address
func (fc *FrontendConfig) GetListenAddr() string {
	// parse the port from the base URL
	u, err := url.Parse(fc.BaseURL)
	if err != nil {
		panic(fmt.Errorf("url.Parse(%q): %w", fc.BaseURL, err))
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		port = 8080
	}
	host := u.Hostname()
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// CacheConfig contains cache-specific configuration
type CacheConfig struct {
	MaxObj     string `yaml:"maxobj"`
	MaxCost    string `yaml:"maxcost"`
	IgnoreHost bool   `yaml:"ignorehost"` // When true, cache keys are generated without considering the host
}

// ParseSize parses a human-readable size into an int64
func ParseSize(size string) int64 {
	var value int64
	var unit string

	n, _ := fmt.Sscanf(size, "%d%s", &value, &unit)
	if n < 1 {
		return 0
	}

	var multiplier int64 = 1
	switch strings.ToUpper(unit) {
	case "K":
		multiplier = 1000
	case "M":
		multiplier = 1000000
	case "G":
		multiplier = 1000000000
	}

	return value * multiplier
}

// GetMaxObjects returns the parsed max objects value
func (cc *CacheConfig) GetMaxObjects() int64 {
	return ParseSize(cc.MaxObj)
}

// GetMaxSize returns the parsed max size value
func (cc *CacheConfig) GetMaxSize() int64 {
	return ParseSize(cc.MaxCost)
}

// GetLogLevel returns the configured log level as a slog.Level
func (c *Config) GetLogLevel() slog.Level {
	switch strings.ToLower(c.Logging.Level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	// Set default values
	cfg := &Config{
		DefaultBackend: BackendConfig{
			Target:  "https://www.varnish-software.com:443",
			Timeout: 30 * time.Second,
		},
		Frontend: FrontendConfig{
			BaseURL:     "http://localhost:8080",
			MetricsPort: 9091,
		},
		Cache: CacheConfig{
			MaxObj:     "1M",
			MaxCost:    "1G",
			IgnoreHost: false, // By default, consider the host in cache keys
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}

	// Read configuration file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Parse YAML configuration
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return cfg, nil
}
