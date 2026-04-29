package storage

import (
	"encoding/json"
	"os"
	"path/filepath"

	"httpaas-manager/models"
)

const InstancesFile = "data/instances.json"

func ensureFileExists(path string) error {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte("[]"), 0644)
	}

	return nil
}

func LoadInstances() ([]models.WebInstance, error) {
	if err := ensureFileExists(InstancesFile); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(InstancesFile)
	if err != nil {
		return nil, err
	}

	var instances []models.WebInstance

	if len(data) == 0 {
		return []models.WebInstance{}, nil
	}

	if err := json.Unmarshal(data, &instances); err != nil {
		return nil, err
	}

	return instances, nil
}

func SaveInstances(instances []models.WebInstance) error {
	if err := ensureFileExists(InstancesFile); err != nil {
		return err
	}

	data, err := json.MarshalIndent(instances, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(InstancesFile, data, 0644)
}

func AddInstance(instance models.WebInstance) (models.WebInstance, error) {
	instances, err := LoadInstances()
	if err != nil {
		return models.WebInstance{}, err
	}

	nextID := 1
	for _, item := range instances {
		if item.ID >= nextID {
			nextID = item.ID + 1
		}
	}

	instance.ID = nextID
	instances = append(instances, instance)

	if err := SaveInstances(instances); err != nil {
		return models.WebInstance{}, err
	}

	return instance, nil
}