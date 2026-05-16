package storage

import (
	"encoding/json"
	"os"
	"time"

	"httpaas-manager/models"
)

const DBInstancesFile = "db_instancias.json"

// MaxDBLogLines caps the in-record pipeline log so the JSON file stays small.
const MaxDBLogLines = 200

func LoadDBInstances() ([]models.DBInstance, error) {
	if err := ensureFileExists(DBInstancesFile); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(DBInstancesFile)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return []models.DBInstance{}, nil
	}

	var instances []models.DBInstance
	if err := json.Unmarshal(data, &instances); err != nil {
		return nil, err
	}
	return instances, nil
}

func SaveDBInstances(instances []models.DBInstance) error {
	if err := ensureFileExists(DBInstancesFile); err != nil {
		return err
	}
	data, err := json.MarshalIndent(instances, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(DBInstancesFile, data, 0644)
}

func AddDBInstance(instance models.DBInstance) (models.DBInstance, error) {
	instances, err := LoadDBInstances()
	if err != nil {
		return models.DBInstance{}, err
	}
	instances = append(instances, instance)
	if err := SaveDBInstances(instances); err != nil {
		return models.DBInstance{}, err
	}
	return instance, nil
}

func UpdateDBInstanceStatus(hostName, status string) (models.DBInstance, bool, error) {
	instances, err := LoadDBInstances()
	if err != nil {
		return models.DBInstance{}, false, err
	}
	for i, ins := range instances {
		if ins.HostName == hostName {
			instances[i].Status = status
			if err := SaveDBInstances(instances); err != nil {
				return models.DBInstance{}, false, err
			}
			return instances[i], true, nil
		}
	}
	return models.DBInstance{}, false, nil
}

func RemoveDBInstanceByHostName(hostName string) (models.DBInstance, bool, error) {
	instances, err := LoadDBInstances()
	if err != nil {
		return models.DBInstance{}, false, err
	}
	out := make([]models.DBInstance, 0, len(instances))
	var removed models.DBInstance
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
		return models.DBInstance{}, false, nil
	}
	if err := SaveDBInstances(out); err != nil {
		return models.DBInstance{}, false, err
	}
	return removed, true, nil
}

// AppendDBInstanceLog appends a timestamped line to the instance log, trimming
// to MaxDBLogLines so the JSON record stays bounded.
func AppendDBInstanceLog(hostName, line string) error {
	instances, err := LoadDBInstances()
	if err != nil {
		return err
	}
	stamp := time.Now().Format("2006-01-02 15:04:05")
	for i, ins := range instances {
		if ins.HostName == hostName {
			entry := stamp + " " + line
			instances[i].Logs = append(instances[i].Logs, entry)
			if len(instances[i].Logs) > MaxDBLogLines {
				instances[i].Logs = instances[i].Logs[len(instances[i].Logs)-MaxDBLogLines:]
			}
			return SaveDBInstances(instances)
		}
	}
	return nil
}
