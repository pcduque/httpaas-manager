package services

import (
	"fmt"
	"os/exec"
)

type SSHConfig struct {
	User       string
	Host       string
	RemotePath string
}

func RunCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func RunSSHCommand(cfg SSHConfig, command string) (string, error) {
	target := fmt.Sprintf("%s@%s", cfg.User, cfg.Host)

	return RunCommand(
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		target,
		command,
	)
}

func CopyFileBySCP(cfg SSHConfig, localPath string) (string, error) {
	target := fmt.Sprintf("%s@%s:%s", cfg.User, cfg.Host, cfg.RemotePath)

	return RunCommand(
		"scp",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		localPath,
		target,
	)
}