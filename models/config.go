package models

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	DataSources struct {
		Parameters []string `json:"parameters"`
		Predict    string   `json:"predict"`
		ModelPath  string   `json:"model_path"`
	} `json:"data_sources"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
	BufferSize          int    `json:"buffer_size"`
	UDSNormalizerPath   string `json:"uds_normalizer_path"`
	UDSSocketPath       string `json:"uds_socket_path"`
}

func NewConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}
	return &config, nil
}