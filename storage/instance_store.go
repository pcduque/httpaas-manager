package storage

import (
	"encoding/json"
	"os"

	"httpaas-manager/models"
)

const InstancesFile = "instancias.json"

func ensureFileExists(path string) error {
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

	if len(data) == 0 {
		return []models.WebInstance{}, nil
	}

	var instances []models.WebInstance
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
	instances = append(instances, instance)
	if err := SaveInstances(instances); err != nil {
		return models.WebInstance{}, err
	}
	return instance, nil
}

func UpdateInstanceStatus(hostName, status string) (models.WebInstance, bool, error) {
	instances, err := LoadInstances()
	if err != nil {
		return models.WebInstance{}, false, err
	}
	for i, ins := range instances {
		if ins.HostName == hostName {
			instances[i].Status = status
			if err := SaveInstances(instances); err != nil {
				return models.WebInstance{}, false, err
			}
			return instances[i], true, nil
		}
	}
	return models.WebInstance{}, false, nil
}

func RemoveInstanceByHostName(hostName string) (models.WebInstance, bool, error) {
	instances, err := LoadInstances()
	if err != nil {
		return models.WebInstance{}, false, err
	}
	out := make([]models.WebInstance, 0, len(instances))
	var removed models.WebInstance
	found := false
	for _, ins := range instances {
		if ins.HostName == hostName {
			removed = ins
			found = true
			continue
		}
		out = append(out, ins)
	}
	if !found {
		return models.WebInstance{}, false, nil
	}
	if err := SaveInstances(out); err != nil {
		return models.WebInstance{}, false, err
	}
	return removed, true, nil
}
