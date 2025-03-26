package main

import (
	"os"
	"testing"

	"github.com/peterldowns/testy/assert"
)

func TestLoadConfig(t *testing.T) {

	cfg := Config{}

	// Ensure loading the configuration fails if there are no env files
	// or command arguments provided.
	err := loadConfig(&cfg, "")
	assert.Error(t, err)

	os.Setenv("collectorgrpcurl", "localhost:4317")
	os.Setenv("collectorhttpurl", "localhost:4318")
	os.Setenv("servicename", "ogen")
	os.Setenv("pprofurl", "localhost:1777")

	err = loadConfig(&cfg, "")
	assert.NoError(t, err)

	// Validate the configuration.
	assert.Equal(t, "localhost:4317", cfg.CollectorGRPCURL)
	assert.Equal(t, "localhost:4318", cfg.CollectorHTTPURL)
	assert.Equal(t, "ogen", cfg.ServiceName)
	assert.Equal(t, "localhost:1777", cfg.PprofURL)

	// Reset the environment variables set.
	os.Unsetenv("pprofport")
	os.Unsetenv("metricsport")
	os.Unsetenv("tracerport")
	os.Unsetenv("loggerport")

	// Load the configuration from an .env file.
	err = loadConfig(&cfg, "data/testenv")
	assert.NoError(t, err)

	// Validate the configuration.
	assert.Equal(t, "localhost:4317", cfg.CollectorGRPCURL)
	assert.Equal(t, "localhost:4318", cfg.CollectorHTTPURL)
	assert.Equal(t, "ogen", cfg.ServiceName)
	assert.Equal(t, "localhost:1777", cfg.PprofURL)
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		hasError bool
	}{
		{
			name: "valid config",
			config: Config{
				CollectorGRPCURL: "localhost:4317",
				CollectorHTTPURL: "localhost:4318",
				ServiceName:      "ogen",
				PprofURL:         "localhost:1777",
			},
			hasError: false,
		},
		{
			name: "missing collector grpc url",
			config: Config{
				CollectorHTTPURL: "localhost:4318",
				ServiceName:      "ogen",
				PprofURL:         "localhost:1777",
			},
			hasError: true,
		},
		{
			name: "missing collector http url",
			config: Config{
				CollectorGRPCURL: "localhost:4317",
				ServiceName:      "ogen",
				PprofURL:         "localhost:1777",
			},
			hasError: true,
		},
		{
			name: "missing serivce name",
			config: Config{
				CollectorGRPCURL: "localhost:4317",
				CollectorHTTPURL: "localhost:4318",
				PprofURL:         "localhost:1777",
			},
			hasError: true,
		},
		{
			name: "missing pprof url",
			config: Config{
				CollectorGRPCURL: "localhost:4317",
				CollectorHTTPURL: "localhost:4318",
				ServiceName:      "ogen",
			},
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
