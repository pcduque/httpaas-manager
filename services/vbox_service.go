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

// CreateVMFromBase provisions a new VM that boots from the multi-attach base
// VDI in cfg.BaseVDIPath. VirtualBox creates a per-VM differencing image
// automatically, so the base disk stays read-only and shared across all
// instances. The base VDI must already be in multi-attach mode AND have a
// owner VM holding a reference (the template prep VM serves that role).
// Engine-agnostic: the caller picks the VDI per workload (Apache, MariaDB,
// PostgreSQL, ...).
func CreateVMFromBase(cfg VMConfig) error {
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

// CreateApacheVM is the legacy alias kept so the existing HTTPaaS handler
// compiles without changes. New code should call CreateVMFromBase directly.
func CreateApacheVM(cfg VMConfig) error {
	return CreateVMFromBase(cfg)
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

// RestartVM does an ACPI shutdown, waits up to 30 s for the guest to power off
// (with a forced poweroff fallback), and then boots it again headless. Used by
// the manager UI for the "reiniciar" action; preserves IP, FQDN, and the DNS
// record because the static config inside the guest is unchanged.
func RestartVM(vmName string) error {
	if out, err := RunVBoxManage("controlvm", vmName, "acpipowerbutton"); err != nil {
		if !strings.Contains(out, "is not currently running") {
			return fmt.Errorf("controlvm acpipowerbutton %s: %v - %s", vmName, err, out)
		}
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if state, _ := VMState(vmName); state == "poweroff" {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if state, _ := VMState(vmName); state != "poweroff" {
		if err := ForcePowerOffVM(vmName); err != nil {
			return fmt.Errorf("force poweroff during restart: %w", err)
		}
		confirmDeadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(confirmDeadline) {
			if state, _ := VMState(vmName); state == "poweroff" {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}

	return StartVM(vmName)
}

// CloneMediumDisk produces a standalone copy of an existing VDI. Used during
// DB-template preparation to fork debian-web1.vdi into debian-mariadb.vdi /
// debian-postgres.vdi before the engine is installed.
func CloneMediumDisk(srcVDI, dstVDI string) error {
	out, err := RunVBoxManage("clonemedium", "disk", srcVDI, dstVDI, "--variant", "Standard")
	if err != nil {
		return fmt.Errorf("clonemedium %s -> %s: %v - %s", srcVDI, dstVDI, err, out)
	}
	return nil
}

// CreateVMWithDirectDisk attaches the VDI as a regular (non-multiattach) disk
// so we can boot once, install the DB engine, and shut down. The result is
// later converted to multiattach via EnsureMultiAttachBase. Mirrors
// CreateVMFromBase but adds a NAT NIC2 because template prep needs outbound
// internet for `apt install`, while the original CreateVMFromBase deliberately
// stays Host-Only (instances don't need internet).
func CreateVMWithDirectDisk(cfg VMConfig) error {
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
		return fmt.Errorf("BaseVDIPath is required")
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
		"--nic2", "nat",
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

// DetachAndUnregisterVM powers off the VM, detaches its disk so the VDI file
// survives, and then unregisters the VM without --delete. Used after the
// one-time template install: the VDI continues to exist on disk and can be
// switched to multi-attach as the new base.
func DetachAndUnregisterVM(vmName string) error {
	if out, err := RunVBoxManage("controlvm", vmName, "poweroff"); err != nil {
		if !strings.Contains(out, "is not currently running") {
			fmt.Printf("warning: poweroff %s: %v - %s\n", vmName, err, out)
		}
	}
	time.Sleep(3 * time.Second)

	if out, err := RunVBoxManage("storageattach", vmName,
		"--storagectl", "SATA",
		"--port", "0",
		"--device", "0",
		"--type", "hdd",
		"--medium", "none"); err != nil {
		fmt.Printf("warning: detach disk %s: %v - %s\n", vmName, err, out)
	}

	if out, err := RunVBoxManage("unregistervm", vmName); err != nil {
		return fmt.Errorf("unregistervm %s: %v - %s", vmName, err, out)
	}
	return nil
}

// CloseMedium drops a VDI registration from VirtualBox without deleting the
// underlying file. Required after detaching a disk if we need to reattach it
// later with a different mode (e.g., multiattach).
func CloseMedium(vdiPath string) error {
	if out, err := RunVBoxManage("closemedium", "disk", vdiPath); err != nil {
		if strings.Contains(out, "not found") || strings.Contains(out, "could not find") {
			return nil
		}
		return fmt.Errorf("closemedium %s: %v - %s", vdiPath, err, out)
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
