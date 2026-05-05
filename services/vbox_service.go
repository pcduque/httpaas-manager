package services

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"httpaas-manager/config"
)

type VMConfig struct {
	VMName      string
	BaseVDIPath string
	HostOnlyIF  string
	MemoryMB    int
	CPUs        int
	OSType      string
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

// CreateApacheVM provisions a new VM that boots from the multi-attach base VDI.
// VirtualBox creates a per-VM differencing image automatically, so the base disk
// stays read-only and shared across all instances.
func CreateApacheVM(cfg VMConfig) error {
	if cfg.HostOnlyIF == "" {
		cfg.HostOnlyIF = config.HostOnlyIF
	}
	if cfg.OSType == "" {
		cfg.OSType = "Debian_64"
	}
	if cfg.MemoryMB == 0 {
		cfg.MemoryMB = 1024
	}
	if cfg.CPUs == 0 {
		cfg.CPUs = 1
	}
	if cfg.BaseVDIPath == "" {
		return fmt.Errorf("BaseVDIPath is required for multi-attach provisioning")
	}

	if out, err := RunVBoxManage("createvm",
		"--name", cfg.VMName,
		"--ostype", cfg.OSType,
		"--register"); err != nil {
		return fmt.Errorf("createvm: %v - %s", err, out)
	}

	if out, err := RunVBoxManage("modifyvm", cfg.VMName,
		"--memory", strconv.Itoa(cfg.MemoryMB),
		"--cpus", strconv.Itoa(cfg.CPUs),
		"--nic1", "hostonly",
		"--hostonlyadapter1", cfg.HostOnlyIF,
		"--boot1", "disk",
		"--boot2", "none",
		"--boot3", "none",
		"--boot4", "none"); err != nil {
		return fmt.Errorf("modifyvm: %v - %s", err, out)
	}

	if out, err := RunVBoxManage("storagectl", cfg.VMName,
		"--name", "SATA",
		"--add", "sata",
		"--controller", "IntelAhci",
		"--portcount", "1"); err != nil {
		return fmt.Errorf("storagectl: %v - %s", err, out)
	}

	if out, err := RunVBoxManage("storageattach", cfg.VMName,
		"--storagectl", "SATA",
		"--port", "0",
		"--device", "0",
		"--type", "hdd",
		"--medium", cfg.BaseVDIPath); err != nil {
		return fmt.Errorf("storageattach: %v - %s", err, out)
	}

	if out, err := RunVBoxManage("startvm", cfg.VMName, "--type", "headless"); err != nil {
		return fmt.Errorf("startvm: %v - %s", err, out)
	}

	return nil
}

// EnsureMultiAttachBase makes sure the base VDI is registered as multi-attach so
// that new VMs receive their own differencing image instead of sharing writes.
// Idempotent: returns changed=false when the medium is already multi-attach.
func EnsureMultiAttachBase(vdiPath string) (bool, error) {
	if vdiPath == "" {
		return false, fmt.Errorf("base VDI path is empty (set BASE_VDI_PATH)")
	}
	out, err := RunVBoxManage("showmediuminfo", "disk", vdiPath)
	if err != nil {
		return false, fmt.Errorf("showmediuminfo: %v - %s", err, out)
	}
	if currentMediumType(out) == "multiattach" {
		return false, nil
	}
	if out, err := RunVBoxManage("modifymedium", "disk", vdiPath, "--type", "multiattach"); err != nil {
		return false, fmt.Errorf("modifymedium: %v - %s", err, out)
	}
	return true, nil
}

func currentMediumType(showOutput string) string {
	for _, line := range strings.Split(showOutput, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "type:") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			return strings.ToLower(strings.TrimSpace(parts[1]))
		}
	}
	return ""
}

// VMState returns the VirtualBox machine state as reported by `showvminfo
// --machinereadable` (for example "running", "poweroff", "saved", "aborted").
// Returns an empty string if the VMState line cannot be located.
func VMState(vmName string) (string, error) {
	out, err := RunVBoxManage("showvminfo", vmName, "--machinereadable")
	if err != nil {
		return "", fmt.Errorf("showvminfo: %v - %s", err, out)
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "VMState=") {
			continue
		}
		return strings.Trim(strings.TrimPrefix(line, "VMState="), `"`), nil
	}
	return "", nil
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

// ForcePowerOffVM yanks the power on a VM whose guest didn't honor the ACPI
// shutdown event (typically because acpid isn't installed in the guest).
// Equivalent to pulling the cable. Returns nil if the VM was already off.
func ForcePowerOffVM(vmName string) error {
	if out, err := RunVBoxManage("controlvm", vmName, "poweroff"); err != nil {
		if strings.Contains(out, "is not currently running") {
			return nil
		}
		return fmt.Errorf("controlvm poweroff %s: %v - %s", vmName, err, out)
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
