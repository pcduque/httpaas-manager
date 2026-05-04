package services

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"httpaas-manager/config"
)

type VMConfig struct {
	VMName         string
	TemplateVMName string
	HostOnlyIF     string
}

type VBoxRunner struct {
	Local          bool
	WindowsHost    string
	WindowsUser    string
	SSHKeyPath     string
	VBoxManagePath string
}

var runner VBoxRunner

func InitVBox(cfg config.Config) {
	runner = VBoxRunner{
		Local:          cfg.VBoxLocal,
		WindowsHost:    cfg.WindowsHost,
		WindowsUser:    cfg.WindowsUser,
		SSHKeyPath:     cfg.SSHKeyPath,
		VBoxManagePath: cfg.VBoxManagePath,
	}
}

// VBoxMode returns a human-readable string describing the active execution mode.
func VBoxMode() string {
	if runner.Local {
		return "local (" + runner.VBoxManagePath + ")"
	}
	return "remote ssh (" + runner.WindowsUser + "@" + runner.WindowsHost + ")"
}

func RunVBoxManage(args ...string) (string, error) {
	return runner.run(args...)
}

func (r VBoxRunner) run(args ...string) (string, error) {
	if r.Local {
		return r.runLocal(args...)
	}
	return r.runRemote(args...)
}

func (r VBoxRunner) runLocal(args ...string) (string, error) {
	binary := r.VBoxManagePath
	if binary == "" {
		binary = "VBoxManage"
	}
	cmd := exec.Command(binary, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r VBoxRunner) runRemote(args ...string) (string, error) {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteWinArg(r.VBoxManagePath))
	for _, a := range args {
		parts = append(parts, quoteWinArg(a))
	}
	remoteCmd := strings.Join(parts, " ")

	sshArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}
	if r.SSHKeyPath != "" {
		sshArgs = append(sshArgs, "-i", r.SSHKeyPath)
	}
	sshArgs = append(sshArgs, fmt.Sprintf("%s@%s", r.WindowsUser, r.WindowsHost), remoteCmd)

	cmd := exec.Command("ssh", sshArgs...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func quoteWinArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	escaped := strings.ReplaceAll(arg, `"`, `\"`)
	return `"` + escaped + `"`
}

// CloneApacheVM clones the template VM, attaches a host-only NIC, and boots it headless.
func CloneApacheVM(cfg VMConfig) error {
	if cfg.HostOnlyIF == "" {
		cfg.HostOnlyIF = config.HostOnlyIF
	}
	if cfg.TemplateVMName == "" {
		cfg.TemplateVMName = config.TemplateVM
	}

	if out, err := RunVBoxManage("clonevm", cfg.TemplateVMName,
		"--name", cfg.VMName, "--register"); err != nil {
		return fmt.Errorf("clonevm: %v - %s", err, out)
	}

	if out, err := RunVBoxManage("modifyvm", cfg.VMName,
		"--nic1", "hostonly",
		"--hostonlyadapter1", cfg.HostOnlyIF); err != nil {
		return fmt.Errorf("modifyvm hostonly: %v - %s", err, out)
	}

	if out, err := RunVBoxManage("startvm", cfg.VMName, "--type", "headless"); err != nil {
		return fmt.Errorf("startvm: %v - %s", err, out)
	}

	return nil
}

// StartVM boots an existing registered VM in headless mode.
func StartVM(vmName string) error {
	if out, err := RunVBoxManage("startvm", vmName, "--type", "headless"); err != nil {
		if strings.Contains(out, "is already locked") || strings.Contains(out, "already running") {
			return nil
		}
		return fmt.Errorf("startvm %s: %v - %s", vmName, err, out)
	}
	return nil
}

// StopVM sends an ACPI shutdown signal so the guest powers off cleanly.
func StopVM(vmName string) error {
	if out, err := RunVBoxManage("controlvm", vmName, "acpipowerbutton"); err != nil {
		if strings.Contains(out, "is not currently running") {
			return nil
		}
		return fmt.Errorf("controlvm acpipowerbutton %s: %v - %s", vmName, err, out)
	}
	return nil
}

// UnregisterVM powers off the VM and removes it together with its disks.
func UnregisterVM(vmName string) error {
	if out, err := RunVBoxManage("controlvm", vmName, "poweroff"); err != nil {
		if !strings.Contains(out, "is not currently running") {
			fmt.Printf("warning: poweroff %s: %v - %s\n", vmName, err, out)
		}
	}

	time.Sleep(3 * time.Second)

	if out, err := RunVBoxManage("unregistervm", vmName, "--delete"); err != nil {
		return fmt.Errorf("unregistervm: %v - %s", err, out)
	}
	return nil
}
