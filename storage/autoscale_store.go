package storage

import (
	"encoding/json"
	"os"

	"haproxy-manager/models"
)

const (
	AutoScaleConfigFile = "data/autoscale_config.json"
	AutoScaleStateFile  = "data/autoscale_state.json"
)

func LoadAutoScaleConfig() (models.AutoScaleConfig, error) {
	if err := ensureFileExists(AutoScaleConfigFile); err != nil {
		return models.AutoScaleConfig{}, err
	}

	data, err := os.ReadFile(AutoScaleConfigFile)
	if err != nil {
		return models.AutoScaleConfig{}, err
	}

	if len(data) == 0 {
		return models.AutoScaleConfig{}, nil
	}

	var cfg models.AutoScaleConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return models.AutoScaleConfig{}, err
	}

	return cfg, nil
}

func SaveAutoScaleConfig(cfg models.AutoScaleConfig) error {
	if err := ensureFileExists(AutoScaleConfigFile); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(AutoScaleConfigFile, data, 0644)
}

func LoadAutoScaleState() (models.AutoScaleState, error) {
	if err := ensureFileExists(AutoScaleStateFile); err != nil {
		return models.AutoScaleState{}, err
	}

	data, err := os.ReadFile(AutoScaleStateFile)
	if err != nil {
		return models.AutoScaleState{}, err
	}

	if len(data) == 0 {
		return models.AutoScaleState{}, nil
	}

	var state models.AutoScaleState
	if err := json.Unmarshal(data, &state); err != nil {
		return models.AutoScaleState{}, err
	}

	return state, nil
}

func SaveAutoScaleState(state models.AutoScaleState) error {
	if err := ensureFileExists(AutoScaleStateFile); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(AutoScaleStateFile, data, 0644)
}