package services

import (
	"fmt"
	"strings"
	"time"

	"haproxy-manager/models"
	"haproxy-manager/storage"
)

func getNextAutoServerNumber(servers []models.BackendServer) int {
	maxNum := 0

	for _, s := range servers {
		if strings.HasPrefix(s.Name, "auto_srv_") {
			var n int
			_, err := fmt.Sscanf(s.Name, "auto_srv_%d", &n)
			if err == nil && n > maxNum {
				maxNum = n
			}
		}
	}

	return maxNum + 1
}

func isHostAlreadyRegistered(host string, servers []models.BackendServer) bool {
	for _, s := range servers {
		if s.IPAddress == host {
			return true
		}
	}
	return false
}

func getNextAvailableHost(cfg models.AutoScaleConfig, currentServers []models.BackendServer) (string, error) {
	for _, host := range cfg.AvailableHosts {
		if !isHostAlreadyRegistered(host, currentServers) {
			return host, nil
		}
	}
	return "", fmt.Errorf("no available hosts left in pool")
}

func registerServerWithHost(cfg models.AutoScaleConfig, host string) (models.BackendServer, error) {
	servers, err := storage.LoadServers()
	if err != nil {
		return models.BackendServer{}, err
	}

	nextID := 1
	for _, s := range servers {
		if s.ID >= nextID {
			nextID = s.ID + 1
		}
	}

	currentBalancerServers := make([]models.BackendServer, 0)
	for _, s := range servers {
		if s.BalancerID == cfg.BalancerID {
			currentBalancerServers = append(currentBalancerServers, s)
		}
	}

	autoNum := getNextAutoServerNumber(currentBalancerServers)

	server := models.BackendServer{
		ID:         nextID,
		BalancerID: cfg.BalancerID,
		Name:       fmt.Sprintf("auto_srv_%d", autoNum),
		IPAddress:  host,
		Port:       cfg.AppPort,
	}

	servers = append(servers, server)

	if err := storage.SaveServers(servers); err != nil {
		return models.BackendServer{}, err
	}

	return server, nil
}

func ManualScaleOut() (map[string]interface{}, error) {
	cfg, err := storage.LoadAutoScaleConfig()
	if err != nil {
		return nil, err
	}

	if err := ValidateAutoScaleConfig(cfg); err != nil {
		return nil, err
	}

	currentServers, err := GetServersByBalancerID(cfg.BalancerID)
	if err != nil {
		return nil, err
	}

	if len(currentServers) >= cfg.MaxInstances {
		return nil, fmt.Errorf("max instances reached")
	}

	host, err := getNextAvailableHost(cfg, currentServers)
	if err != nil {
		return nil, err
	}

	server, err := registerServerWithHost(cfg, host)
	if err != nil {
		return nil, err
	}

	content, filePath, err := BuildAndSaveFullHAProxyConfig()
	if err != nil {
		return nil, err
	}

	sshCfg := SSHConfig{
		User:       "server1",
		Host:       "192.168.1.6",
		RemotePath: "/home/server1",
	}

	if err := ApplyRemoteHAProxyConfig(sshCfg, filePath); err != nil {
		return nil, err
	}

	state, _ := storage.LoadAutoScaleState()
	state.LastAction = "manual_scale_out_executed"
	state.LastActionTime = time.Now().Format(time.RFC3339)
	state.ActiveInstances = len(currentServers) + 1
	_ = storage.SaveAutoScaleState(state)

	return map[string]interface{}{
		"message":    "manual scale out executed successfully",
		"added_host": host,
		"server":     server,
		"config":     content,
	}, nil
}

func ManualScaleIn() (map[string]interface{}, error) {
	cfg, err := storage.LoadAutoScaleConfig()
	if err != nil {
		return nil, err
	}

	if err := ValidateAutoScaleConfig(cfg); err != nil {
		return nil, err
	}

	currentServers, err := GetServersByBalancerID(cfg.BalancerID)
	if err != nil {
		return nil, err
	}

	if len(currentServers) <= cfg.MinInstances {
		return nil, fmt.Errorf("min instances reached")
	}

	var removable *models.BackendServer
	for i := len(currentServers) - 1; i >= 0; i-- {
		if strings.HasPrefix(currentServers[i].Name, "auto_srv_") {
			removable = &currentServers[i]
			break
		}
	}

	if removable == nil {
		return nil, fmt.Errorf("no auto-added server available to remove")
	}

	allServers, err := storage.LoadServers()
	if err != nil {
		return nil, err
	}

	filtered := make([]models.BackendServer, 0)
	for _, s := range allServers {
		if s.ID != removable.ID {
			filtered = append(filtered, s)
		}
	}

	if err := storage.SaveServers(filtered); err != nil {
		return nil, err
	}

	content, filePath, err := BuildAndSaveFullHAProxyConfig()
	if err != nil {
		return nil, err
	}

	sshCfg := SSHConfig{
		User:       "server1",
		Host:       "192.168.1.6",
		RemotePath: "/home/server1",
	}

	if err := ApplyRemoteHAProxyConfig(sshCfg, filePath); err != nil {
		return nil, err
	}

	state, _ := storage.LoadAutoScaleState()
	state.LastAction = "manual_scale_in_executed"
	state.LastActionTime = time.Now().Format(time.RFC3339)
	state.ActiveInstances = len(currentServers) - 1
	_ = storage.SaveAutoScaleState(state)

	return map[string]interface{}{
		"message":        "manual scale in executed successfully",
		"removed_server": removable,
		"config":         content,
	}, nil
}