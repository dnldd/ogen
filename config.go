package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

var registeredFlags = make(map[string]bool)

// registeredFlag registers command line arguments and tracks them to avoid reregistration.
func registerFlag(name string, value *string, usage string) error {
	defaultValue := os.Getenv(name)

	if !registeredFlags[name] {
		flag.StringVar(value, name, defaultValue, usage)
		registeredFlags[name] = true
	}

	if registeredFlags[name] && defaultValue != "" {
		*value = defaultValue
	}

	return nil
}

// Config is the configuration struct for the service.
type Config struct {
	CollectorGRPCURL string
	CollectorHTTPURL string
	ServiceName      string
	PprofURL         string
	MetricsPort      uint64
	TracerPort       uint64
	LoggerPort       uint64
}

// validate ensures that the configuration is valid.
func (c *Config) validate() error {
	var errs error

	if c.CollectorGRPCURL == "" {
		errs = errors.Join(errs, fmt.Errorf("collector grpc endpoint required"))
	}

	if c.CollectorHTTPURL == "" {
		errs = errors.Join(errs, fmt.Errorf("collector http endpoint required"))
	}

	if c.ServiceName == "" {
		errs = errors.Join(errs, fmt.Errorf("service name required"))
	}

	if c.PprofURL == "" {
		errs = errors.Join(errs, fmt.Errorf("pprof endpoint required"))
	}

	return errs
}

// loadConfig loads the configuration from environment variables and command line flags.
func loadConfig(cfg *Config, path string) error {
	if path == "" {
		path = ".env"
	}

	// Check if the expected .env file exists before loading it.
	_, err := os.Stat(path)
	if err == nil {
		err := godotenv.Load(path)
		if err != nil {
			return fmt.Errorf("loading .env file: %w", err)
		}
	}

	// Register command line arguments using loaded environment variables as defaults.
	registerFlag("collectorgrpcurl", &cfg.CollectorGRPCURL, "the collector grpc endpoint")
	registerFlag("collectorhttpurl", &cfg.CollectorHTTPURL, "the collector http endpoint")
	registerFlag("servicename", &cfg.ServiceName, "the service name")
	registerFlag("pprofurl", &cfg.PprofURL, "the pprof endpoint")

	// Parse command-line flags.
	flag.Parse()

	return cfg.validate()
}
