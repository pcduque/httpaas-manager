package storage

import (
	"encoding/json"
	"os"
	"path/filepath"

	"haproxy-manager/models"
)

const (
	BalancersFile = "data/balancers.json"
	ServersFile   = "data/servers.json"
)

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

func LoadBalancers() ([]models.Balancer, error) {
	if err := ensureFileExists(BalancersFile); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(BalancersFile)
	if err != nil {
		return nil, err
	}

	var balancers []models.Balancer
	if len(data) == 0 {
		return []models.Balancer{}, nil
	}

	if err := json.Unmarshal(data, &balancers); err != nil {
		return nil, err
	}

	return balancers, nil
}

func SaveBalancers(balancers []models.Balancer) error {
	if err := ensureFileExists(BalancersFile); err != nil {
		return err
	}

	data, err := json.MarshalIndent(balancers, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(BalancersFile, data, 0644)
}

func LoadServers() ([]models.BackendServer, error) {
	if err := ensureFileExists(ServersFile); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(ServersFile)
	if err != nil {
		return nil, err
	}

	var servers []models.BackendServer
	if len(data) == 0 {
		return []models.BackendServer{}, nil
	}

	if err := json.Unmarshal(data, &servers); err != nil {
		return nil, err
	}

	return servers, nil
}

func SaveServers(servers []models.BackendServer) error {
	if err := ensureFileExists(ServersFile); err != nil {
		return err
	}

	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ServersFile, data, 0644)
}