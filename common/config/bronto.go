package config

import (
	"errors"
	"fmt"

	"github.com/odigos-io/odigos/common"
)

const (
	brontoRegion     = "BRONTO_REGION"
	brontoCollection = "BRONTO_COLLECTION"
)

var (
	ErrorBrontoRegionMissing = errors.New("Bronto is missing a required field (\"BRONTO_REGION\"), Bronto will not be configured")
	ErrorBrontoRegionInvalid = errors.New("Bronto region must be either \"eu\" or \"us\", Bronto will not be configured")
)

type Bronto struct{}

func (b *Bronto) DestType() common.DestinationType {
	return common.BrontoDestinationType
}

func (b *Bronto) ModifyConfig(dest ExporterConfigurer, currentConfig *Config) ([]string, error) {
	config := dest.GetConfig()
	uniqueUri := "bronto-" + dest.GetID()

	region, exists := config[brontoRegion]
	if !exists || region == "" {
		return nil, ErrorBrontoRegionMissing
	}
	if region != "eu" && region != "us" {
		return nil, ErrorBrontoRegionInvalid
	}

	// Bronto exposes a separate OTLP path per signal, all on port 443.
	baseUrl := fmt.Sprintf("https://ingestion.%s.bronto.io", region)

	headers := GenericMap{
		"x-bronto-api-key": "${BRONTO_API_KEY}",
	}
	// Optional: group everything this cluster sends under one Bronto collection,
	// overriding service.namespace.
	if collection, ok := config[brontoCollection]; ok && collection != "" {
		headers["x-bronto-collection"] = collection
	}

	exporterName := "otlp_http/" + uniqueUri
	currentConfig.Exporters[exporterName] = GenericMap{
		"traces_endpoint":  baseUrl + "/v1/traces",
		"metrics_endpoint": baseUrl + "/v1/metrics",
		"logs_endpoint":    baseUrl + "/v1/logs",
		"compression":      "gzip",
		"headers":          headers,
	}

	var pipelineNames []string

	if isTracingEnabled(dest) {
		pipeName := "traces/" + uniqueUri
		currentConfig.Service.Pipelines[pipeName] = Pipeline{
			Exporters: []string{exporterName},
		}
		pipelineNames = append(pipelineNames, pipeName)
	}

	if isMetricsEnabled(dest) {
		// Bronto accepts delta or cumulative, but its metric functions are built
		// around delta and it provides no rate function, so convert before export.
		processorName := "cumulativetodelta/" + uniqueUri
		currentConfig.Processors[processorName] = GenericMap{
			"initial_value": "drop",
		}

		pipeName := "metrics/" + uniqueUri
		currentConfig.Service.Pipelines[pipeName] = Pipeline{
			Processors: []string{processorName},
			Exporters:  []string{exporterName},
		}
		pipelineNames = append(pipelineNames, pipeName)
	}

	if isLoggingEnabled(dest) {
		pipeName := "logs/" + uniqueUri
		currentConfig.Service.Pipelines[pipeName] = Pipeline{
			Exporters: []string{exporterName},
		}
		pipelineNames = append(pipelineNames, pipeName)
	}

	return pipelineNames, nil
}
