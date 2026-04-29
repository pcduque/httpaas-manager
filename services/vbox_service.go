package services

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const VBoxManagePath = `C:\Program Files\Oracle\VirtualBox\VBoxManage.exe`

type VMConfig struct {
	VMName            string
	TemplateVMName    string
	BridgeAdapterName string
}

func RunVBoxManage(args ...string) (string, error) {
	cmd := exec.Command(VBoxManagePath, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func CreateApacheVM(cfg VMConfig) error {
	if cfg.BridgeAdapterName == "" {
		cfg.BridgeAdapterName = "Intel(R) Dual Band Wireless-AC 8265"
	}

	sourceDisk := fmt.Sprintf(
		"C:/Users/Lenovo/VirtualBox VMs/%s/%s.vdi",
		cfg.TemplateVMName,
		cfg.TemplateVMName,
	)

	targetDisk := fmt.Sprintf(
		"C:/Users/Lenovo/VirtualBox VMs/%s.vdi",
		cfg.VMName,
	)

	if output, err := RunVBoxManage("createvm", "--name", cfg.VMName, "--register"); err != nil {
		return fmt.Errorf("error creating VM: %v - output: %s", err, output)
	}

	if output, err := RunVBoxManage("modifyvm", cfg.VMName, "--memory", "1024", "--ostype", "Debian_64", "--cpus", "1"); err != nil {
		return fmt.Errorf("error configuring VM: %v - output: %s", err, output)
	}

	if output, err := RunVBoxManage("modifyvm", cfg.VMName, "--nic1", "bridged", "--bridgeadapter1", cfg.BridgeAdapterName); err != nil {
		return fmt.Errorf("error configuring network: %v - output: %s", err, output)
	}

	if output, err := RunVBoxManage("storagectl", cfg.VMName, "--name", "SATA", "--add", "sata", "--controller", "IntelAhci"); err != nil {
		return fmt.Errorf("error creating storage controller: %v - output: %s", err, output)
	}

	if output, err := RunVBoxManage("clonemedium", "disk", sourceDisk, targetDisk, "--format", "VDI"); err != nil {
		return fmt.Errorf("error cloning disk: %v - output: %s", err, output)
	}

	if output, err := RunVBoxManage("modifymedium", targetDisk, "--type", "multiattach"); err != nil {
		return fmt.Errorf("error setting disk multiattach: %v - output: %s", err, output)
	}

	if output, err := RunVBoxManage(
		"storageattach", cfg.VMName,
		"--storagectl", "SATA",
		"--port", "0",
		"--device", "0",
		"--type", "hdd",
		"--medium", targetDisk,
	); err != nil {
		return fmt.Errorf("error attaching disk: %v - output: %s", err, output)
	}

	if output, err := RunVBoxManage("startvm", cfg.VMName, "--type", "headless"); err != nil {
		return fmt.Errorf("error starting VM: %v - output: %s", err, output)
	}

	return nil
}

func GetVMIPAddress(vmName string) (string, error) {
	for i := 0; i < 20; i++ {
		output, err := RunVBoxManage("guestproperty", "get", vmName, "/VirtualBox/GuestInfo/Net/0/V4/IP")
		if err == nil && strings.Contains(output, "Value:") {
			parts := strings.Split(output, "Value:")
			if len(parts) == 2 {
				ip := strings.TrimSpace(parts[1])
				if ip != "" && ip != "no value set!" {
					return ip, nil
				}
			}
		}

		time.Sleep(5 * time.Second)
	}

	return "", fmt.Errorf("could not get IP for VM %s", vmName)
}