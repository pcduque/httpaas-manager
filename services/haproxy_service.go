package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"haproxy-manager/models"
	"haproxy-manager/storage"
)

func GetBalancerByID(id int) (*models.Balancer, error) {
	balancers, err := storage.LoadBalancers()
	if err != nil {
		return nil, err
	}

	for _, b := range balancers {
		if b.ID == id {
			balancer := b
			return &balancer, nil
		}
	}

	return nil, fmt.Errorf("balancer with id %d not found", id)
}

func GetAllBalancers() ([]models.Balancer, error) {
	return storage.LoadBalancers()
}

func GetServersByBalancerID(balancerID int) ([]models.BackendServer, error) {
	servers, err := storage.LoadServers()
	if err != nil {
		return nil, err
	}

	result := make([]models.BackendServer, 0)
	for _, s := range servers {
		if s.BalancerID == balancerID {
			result = append(result, s)
		}
	}

	return result, nil
}

func validateBalancersForHAProxy(balancers []models.Balancer) error {
	if len(balancers) == 0 {
		return fmt.Errorf("no balancers found")
	}

	usedPorts := make(map[int]bool)

	for _, b := range balancers {
		if b.FrontendPort <= 0 {
			return fmt.Errorf("balancer %d has invalid frontend port", b.ID)
		}

		if usedPorts[b.FrontendPort] {
			return fmt.Errorf("duplicate frontend port detected: %d", b.FrontendPort)
		}
		usedPorts[b.FrontendPort] = true
	}

	return nil
}

func GenerateSingleBalancerBlock(balancer models.Balancer, servers []models.BackendServer) (string, error) {
	if len(servers) == 0 {
		return "", fmt.Errorf("balancer %d has no backend servers", balancer.ID)
	}

	frontendName := fmt.Sprintf("lb_%d_front", balancer.ID)
	backendName := fmt.Sprintf("lb_%d_back", balancer.ID)

	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("frontend %s\n", frontendName))
	builder.WriteString(fmt.Sprintf("    bind *:%d\n", balancer.FrontendPort))
	builder.WriteString(fmt.Sprintf("    default_backend %s\n\n", backendName))

	builder.WriteString(fmt.Sprintf("backend %s\n", backendName))
	builder.WriteString(fmt.Sprintf("    balance %s\n", balancer.Algorithm))
	builder.WriteString("    option tcp-check\n")

	for _, server := range servers {
		builder.WriteString(
			fmt.Sprintf(
				"    server %s %s:%d check inter 3s fall 2 rise 2\n",
				server.Name,
				server.IPAddress,
				server.Port,
			),
		)
	}

	builder.WriteString("\n")
	return builder.String(), nil
}

func GenerateFullHAProxyConfig() (string, error) {
	balancers, err := GetAllBalancers()
	if err != nil {
		return "", err
	}

	if err := validateBalancersForHAProxy(balancers); err != nil {
		return "", err
	}

	var builder strings.Builder

	builder.WriteString("global\n")
	builder.WriteString("    log /dev/log local0\n")
	builder.WriteString("    log /dev/log local1 notice\n")
	builder.WriteString("    daemon\n")
	builder.WriteString("    maxconn 256\n\n")

	builder.WriteString("defaults\n")
	builder.WriteString("    log global\n")
	builder.WriteString("    mode http\n")
	builder.WriteString("    option httplog\n")
	builder.WriteString("    timeout connect 5000\n")
	builder.WriteString("    timeout client 50000\n")
	builder.WriteString("    timeout server 50000\n\n")

	for _, balancer := range balancers {
		servers, err := GetServersByBalancerID(balancer.ID)
		if err != nil {
			return "", err
		}

		block, err := GenerateSingleBalancerBlock(balancer, servers)
		if err != nil {
			return "", err
		}

		builder.WriteString(block)
	}

	return builder.String(), nil
}

func SaveHAProxyConfig(content string) (string, error) {
	dir := "generated"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, "haproxy.cfg")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}

	return path, nil
}

func BuildAndSaveFullHAProxyConfig() (string, string, error) {
	content, err := GenerateFullHAProxyConfig()
	if err != nil {
		return "", "", err
	}

	path, err := SaveHAProxyConfig(content)
	if err != nil {
		return "", "", err
	}

	return content, path, nil
}