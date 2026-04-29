package services

import "fmt"

type StressRequest struct {
	Host        string `json:"host"`
	SSHUser     string `json:"ssh_user"`
	CPUWorkers  int    `json:"cpu_workers"`
	TimeoutSecs int    `json:"timeout_secs"`
}

func StartStress(req StressRequest) (string, error) {
	if req.Host == "" {
		return "", fmt.Errorf("host is required")
	}
	if req.SSHUser == "" {
		req.SSHUser = "server1"
	}
	if req.CPUWorkers <= 0 {
		req.CPUWorkers = 1
	}
	if req.TimeoutSecs <= 0 {
		req.TimeoutSecs = 30
	}

	cfg := SSHConfig{
		User: req.SSHUser,
		Host: req.Host,
	}

	cmd := fmt.Sprintf(
		"nohup stress-ng --cpu %d --timeout %ds >/tmp/stress.log 2>&1 &",
		req.CPUWorkers,
		req.TimeoutSecs,
	)

	output, err := RunSSHCommand(cfg, cmd)
	if err != nil {
		return output, err
	}

	return "stress started successfully", nil
}

func StopStress(req StressRequest) (string, error) {
	if req.Host == "" {
		return "", fmt.Errorf("host is required")
	}
	if req.SSHUser == "" {
		req.SSHUser = "server1"
	}

	cfg := SSHConfig{
		User: req.SSHUser,
		Host: req.Host,
	}

	output, err := RunSSHCommand(cfg, "pkill stress-ng")
	if err != nil {
		return output, err
	}

	return "stress stopped successfully", nil
}