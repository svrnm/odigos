package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newBrontoTestConfig() *Config {
	return &Config{
		Exporters:  GenericMap{},
		Processors: GenericMap{},
		Extensions: GenericMap{},
		Service: Service{
			Extensions: []string{},
			Pipelines:  make(map[string]Pipeline),
		},
	}
}

func TestBrontoRegionValidation(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]string
		wantErr  error
		wantHost string
	}{
		{
			name:     "eu region",
			config:   map[string]string{brontoRegion: "eu"},
			wantHost: "https://ingestion.eu.bronto.io/v1/traces",
		},
		{
			name:     "us region",
			config:   map[string]string{brontoRegion: "us"},
			wantHost: "https://ingestion.us.bronto.io/v1/traces",
		},
		{
			name:    "missing region",
			config:  map[string]string{},
			wantErr: ErrorBrontoRegionMissing,
		},
		{
			name:    "empty region",
			config:  map[string]string{brontoRegion: ""},
			wantErr: ErrorBrontoRegionMissing,
		},
		{
			name:    "unknown region is rejected rather than silently building a bad URL",
			config:  map[string]string{brontoRegion: "ap"},
			wantErr: ErrorBrontoRegionInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentConfig := newBrontoTestConfig()
			dest := &mockDestination{id: "test-id", config: tt.config}

			pipelines, err := (&Bronto{}).ModifyConfig(dest, currentConfig)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, pipelines)
				assert.Empty(t, currentConfig.Exporters)
				return
			}

			assert.NoError(t, err)
			exporter := currentConfig.Exporters["otlp_http/bronto-test-id"].(GenericMap)
			assert.Equal(t, tt.wantHost, exporter["traces_endpoint"])
		})
	}
}

func TestBrontoExporterAndPipelines(t *testing.T) {
	currentConfig := newBrontoTestConfig()
	dest := &mockDestination{
		id:     "test-id",
		config: map[string]string{brontoRegion: "eu"},
	}

	pipelines, err := (&Bronto{}).ModifyConfig(dest, currentConfig)
	assert.NoError(t, err)

	// mockDestination enables all three signals.
	assert.ElementsMatch(t, []string{
		"traces/bronto-test-id",
		"metrics/bronto-test-id",
		"logs/bronto-test-id",
	}, pipelines)

	exporter := currentConfig.Exporters["otlp_http/bronto-test-id"].(GenericMap)
	assert.Equal(t, "https://ingestion.eu.bronto.io/v1/metrics", exporter["metrics_endpoint"])
	assert.Equal(t, "https://ingestion.eu.bronto.io/v1/logs", exporter["logs_endpoint"])
	assert.Equal(t, "gzip", exporter["compression"])

	// The API key must be referenced as a secret env var, never inlined.
	headers := exporter["headers"].(GenericMap)
	assert.Equal(t, "${BRONTO_API_KEY}", headers["x-bronto-api-key"])
	assert.NotContains(t, headers, "x-bronto-collection")

	// Bronto's metric functions assume delta temporality, so the metrics
	// pipeline must convert before export.
	processorName := "cumulativetodelta/bronto-test-id"
	assert.Contains(t, currentConfig.Processors, processorName)
	assert.Equal(t, []string{processorName}, currentConfig.Service.Pipelines["metrics/bronto-test-id"].Processors)

	// Traces and logs are exported as-is.
	assert.Empty(t, currentConfig.Service.Pipelines["traces/bronto-test-id"].Processors)
	assert.Empty(t, currentConfig.Service.Pipelines["logs/bronto-test-id"].Processors)
}

func TestBrontoOptionalCollectionHeader(t *testing.T) {
	currentConfig := newBrontoTestConfig()
	dest := &mockDestination{
		id: "test-id",
		config: map[string]string{
			brontoRegion:     "eu",
			brontoCollection: "prod-eu-west-1",
		},
	}

	_, err := (&Bronto{}).ModifyConfig(dest, currentConfig)
	assert.NoError(t, err)

	exporter := currentConfig.Exporters["otlp_http/bronto-test-id"].(GenericMap)
	headers := exporter["headers"].(GenericMap)
	assert.Equal(t, "prod-eu-west-1", headers["x-bronto-collection"])
}
