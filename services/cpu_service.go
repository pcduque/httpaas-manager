package services

import (
	"fmt"
	"strconv"
	"strings"

	"haproxy-manager/models"
)

func parseCPUStatLine(line string) (idle uint64, total uint64, err error) {
	fields := strings.Fields(line)
	if len(fields) < 8 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("invalid /proc/stat cpu line")
	}

	values := make([]uint64, 0, len(fields)-1)
	for _, f := range fields[1:] {
		v, convErr := strconv.ParseUint(f, 10, 64)
		if convErr != nil {
			return 0, 0, convErr
		}
		values = append(values, v)
	}

	for _, v := range values {
		total += v
	}

	idle = values[3]
	if len(values) > 4 {
		idle += values[4]
	}

	return idle, total, nil
}

func extractCPUStatLines(output string) []string {
	lines := strings.Split(output, "\n")
	result := make([]string, 0, 2)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "cpu ") {
			result = append(result, line)
		}
	}

	return result
}

func readRemoteCPUStat(cfg SSHConfig) (idle uint64, total uint64, err error) {
	output, err := RunSSHCommand(cfg, "cat /proc/stat | head -n 1")
	if err != nil {
		return 0, 0, err
	}

	cpuLines := extractCPUStatLines(output)
	if len(cpuLines) == 0 {
		return 0, 0, fmt.Errorf("no valid cpu line found in output: %s", output)
	}

	return parseCPUStatLine(cpuLines[0])
}

func GetRemoteCPUUsage(cfg SSHConfig) (float64, error) {
	output, err := RunSSHCommand(cfg, "sh -c 'cat /proc/stat | head -n 1; sleep 1; cat /proc/stat | head -n 1'")
	if err != nil {
		return 0, err
	}

	cpuLines := extractCPUStatLines(output)
	if len(cpuLines) < 2 {
		return 0, fmt.Errorf("could not read two valid cpu samples from output: %s", output)
	}

	idle1, total1, err := parseCPUStatLine(cpuLines[0])
	if err != nil {
		return 0, err
	}

	idle2, total2, err := parseCPUStatLine(cpuLines[1])
	if err != nil {
		return 0, err
	}

	deltaIdle := float64(idle2 - idle1)
	deltaTotal := float64(total2 - total1)

	if deltaTotal <= 0 {
		return 0, fmt.Errorf("invalid cpu delta")
	}

	usage := 100.0 * (deltaTotal - deltaIdle) / deltaTotal

	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}

	return usage, nil
}

func GetBalancerAverageCPU(balancerID int, sshUser string) (float64, []map[string]interface{}, error) {
	servers, err := GetServersByBalancerID(balancerID)
	if err != nil {
		return 0, nil, err
	}

	if len(servers) == 0 {
		return 0, nil, fmt.Errorf("balancer %d has no servers", balancerID)
	}

	totalUsage := 0.0
	details := make([]map[string]interface{}, 0, len(servers))

	for _, server := range servers {
		cfg := SSHConfig{
			User: sshUser,
			Host: server.IPAddress,
		}

		usage, usageErr := GetRemoteCPUUsage(cfg)
		if usageErr != nil {
			return 0, nil, fmt.Errorf("error reading cpu from %s: %w", server.IPAddress, usageErr)
		}

		totalUsage += usage
		details = append(details, map[string]interface{}{
			"server_id":   server.ID,
			"name":        server.Name,
			"ip_address":  server.IPAddress,
			"port":        server.Port,
			"cpu_usage":   usage,
			"balancer_id": server.BalancerID,
		})
	}

	avg := totalUsage / float64(len(servers))
	return avg, details, nil
}

func GetConfiguredAverageCPU(cfg models.AutoScaleConfig) (float64, []map[string]interface{}, error) {
	if cfg.BalancerID <= 0 {
		return 0, nil, fmt.Errorf("autoscale balancer_id is required")
	}

	sshUser := "server1"
	if strings.TrimSpace(cfg.SSHUser) != "" {
		sshUser = cfg.SSHUser
	}

	return GetBalancerAverageCPU(cfg.BalancerID, sshUser)
}