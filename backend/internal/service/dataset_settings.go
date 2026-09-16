package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	DatasetGoogleDriveCredentialsMaxBytes = 64 * 1024

	datasetBatchSizeMin      = 1
	datasetBatchSizeMax      = 1000
	datasetBatchMaxMBMin     = 1
	datasetBatchMaxMBMax     = 100
	datasetBatchIntervalMin  = 1
	datasetBatchIntervalMax  = 24 * 60 * 60
	datasetBufferMaxItemsMin = 1
	datasetBufferMaxItemsMax = 100000
)

// ValidateDatasetGoogleDriveCredentials validates the OAuth client JSON accepted
// by google.ConfigFromJSON. An empty value is valid because it means clear the
// stored credential; callers must not use this function to validate a missing
// credential.
func ValidateDatasetGoogleDriveCredentials(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len([]byte(value)) > DatasetGoogleDriveCredentialsMaxBytes {
		return fmt.Errorf("must be at most %d bytes", DatasetGoogleDriveCredentialsMaxBytes)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &root); err != nil {
		return fmt.Errorf("must be valid JSON: %w", err)
	}
	if len(root) == 0 {
		return fmt.Errorf("must contain an installed or web OAuth client configuration")
	}

	for _, kind := range []string{"installed", "web"} {
		rawConfig, ok := root[kind]
		if !ok {
			continue
		}
		var config struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			AuthURI      string `json:"auth_uri"`
			TokenURI     string `json:"token_uri"`
		}
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return fmt.Errorf("%s OAuth client configuration must be an object", kind)
		}
		if strings.TrimSpace(config.ClientID) == "" ||
			strings.TrimSpace(config.ClientSecret) == "" ||
			strings.TrimSpace(config.AuthURI) == "" ||
			strings.TrimSpace(config.TokenURI) == "" {
			return fmt.Errorf("%s OAuth client configuration is missing required fields", kind)
		}
		return nil
	}

	return fmt.Errorf("must contain an installed or web OAuth client configuration")
}

// ValidateDatasetSettingsValues validates the bounded worker/collector values.
func ValidateDatasetSettingsValues(batchSize, batchMaxMB, batchIntervalSec, bufferMaxItems int) error {
	if err := ValidateDatasetBatchSize(batchSize); err != nil {
		return err
	}
	if err := ValidateDatasetBatchMaxMB(batchMaxMB); err != nil {
		return err
	}
	if err := ValidateDatasetBatchIntervalSec(batchIntervalSec); err != nil {
		return err
	}
	return ValidateDatasetBufferMaxItems(bufferMaxItems)
}

// ValidateDatasetBatchSize validates the number of records sent per upload.
func ValidateDatasetBatchSize(value int) error {
	return validateDatasetRange("batch_size", value, datasetBatchSizeMin, datasetBatchSizeMax)
}

// ValidateDatasetBatchMaxMB validates the maximum upload size in megabytes.
func ValidateDatasetBatchMaxMB(value int) error {
	return validateDatasetRange("batch_max_mb", value, datasetBatchMaxMBMin, datasetBatchMaxMBMax)
}

// ValidateDatasetBatchIntervalSec validates the worker flush interval.
func ValidateDatasetBatchIntervalSec(value int) error {
	return validateDatasetRange("batch_interval_sec", value, datasetBatchIntervalMin, datasetBatchIntervalMax)
}

// ValidateDatasetBufferMaxItems validates the in-memory collector capacity.
func ValidateDatasetBufferMaxItems(value int) error {
	return validateDatasetRange("buffer_max_items", value, datasetBufferMaxItemsMin, datasetBufferMaxItemsMax)
}

func validateDatasetRange(name string, value, min, max int) error {
	if value < min || value > max {
		return fmt.Errorf("%s must be between %d and %d", name, min, max)
	}
	return nil
}
