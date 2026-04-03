package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// GatewayConfig is the top-level configuration for the inference gateway.
type GatewayConfig struct {
	ListenAddr  string            `yaml:"listen_addr"`
	Backends    []BackendConfig   `yaml:"backends"`
	Router      RouterConfig      `yaml:"router"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
}

// BackendConfig defines a single backend inference service instance.
type BackendConfig struct {
	ID             string `yaml:"id"`
	Address        string `yaml:"address"`
	MaxConcurrency int    `yaml:"max_concurrency"`
}

// HealthCheckConfig controls how the gateway probes backend health.
type HealthCheckConfig struct {
	IntervalSeconds int `yaml:"interval_seconds"`
	TimeoutSeconds  int `yaml:"timeout_seconds"`
}

// RouterConfig controls routing strategy behavior.
type RouterConfig struct {
	Strategy             string  `yaml:"strategy"`
	PrefixMinLength      int     `yaml:"prefix_min_length"`
	LoadThresholdPercent float64 `yaml:"load_threshold_percent"`
	PrefixCacheMaxSize   int     `yaml:"prefix_cache_max_size"`
}

// Load reads a YAML config file from the given path and returns a GatewayConfig.
func Load(path string) (*GatewayConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &GatewayConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	setDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

func setDefaults(cfg *GatewayConfig) {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.Router.Strategy == "" {
		cfg.Router.Strategy = "hybrid"
	}
	if cfg.Router.PrefixMinLength <= 0 {
		cfg.Router.PrefixMinLength = 4
	}
	if cfg.Router.LoadThresholdPercent <= 0 {
		cfg.Router.LoadThresholdPercent = 0.8
	}
	if cfg.Router.PrefixCacheMaxSize <= 0 {
		cfg.Router.PrefixCacheMaxSize = 10000
	}
	for i := range cfg.Backends {
		if cfg.Backends[i].MaxConcurrency <= 0 {
			cfg.Backends[i].MaxConcurrency = 10
		}
	}
	if cfg.HealthCheck.IntervalSeconds <= 0 {
		cfg.HealthCheck.IntervalSeconds = 5
	}
	if cfg.HealthCheck.TimeoutSeconds <= 0 {
		cfg.HealthCheck.TimeoutSeconds = 2
	}
}

func validate(cfg *GatewayConfig) error {
	if len(cfg.Backends) == 0 {
		return fmt.Errorf("at least one backend must be configured")
	}

	ids := make(map[string]struct{}, len(cfg.Backends))
	for _, b := range cfg.Backends {
		if b.ID == "" {
			return fmt.Errorf("backend id must not be empty")
		}
		if b.Address == "" {
			return fmt.Errorf("backend %q address must not be empty", b.ID)
		}
		if _, exists := ids[b.ID]; exists {
			return fmt.Errorf("duplicate backend id: %q", b.ID)
		}
		ids[b.ID] = struct{}{}
	}

	switch cfg.Router.Strategy {
	case "prefix", "load", "hybrid":
	default:
		return fmt.Errorf("unknown router strategy: %q (must be prefix, load, or hybrid)", cfg.Router.Strategy)
	}

	if cfg.Router.LoadThresholdPercent <= 0 || cfg.Router.LoadThresholdPercent > 1.0 {
		return fmt.Errorf("load_threshold_percent must be in (0, 1.0], got %f", cfg.Router.LoadThresholdPercent)
	}

	return nil
}
