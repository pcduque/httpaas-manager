package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"httpaas-manager/config"
	"httpaas-manager/models"
	"httpaas-manager/services"
	"httpaas-manager/storage"
	"httpaas-manager/utils"
)

var Cfg config.Config

// provisionMu serializes the boot-time configuration of new VMs because every
// fresh instance comes up on Cfg.TemplateInitialIP before being reassigned its
// final static IP. Concurrent provisioning would collide on that shared IP.
var provisionMu sync.Mutex

// allocateMu protects the read-then-save sequence used to reserve the next
// static IP and persist the placeholder record so two concurrent creates
// cannot grab the same IP.
var allocateMu sync.Mutex

func Init(cfg config.Config) {
	Cfg = cfg
}

func ListInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := storage.LoadInstances()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.WriteJSON(w, http.StatusOK, instances)
}

// CreateInstance stages the upload synchronously, persists a "creating"
// placeholder, and runs the slow VM provisioning in a goroutine so the HTTP
// response returns immediately and the UI can show the new instance with a
// transient status badge.
func CreateInstance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(200 << 20); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	hostName := strings.TrimSpace(r.FormValue("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}
	hostName = sanitizeHostName(hostName)
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is invalid")
		return
	}
	vmName := hostName

	file, header, err := r.FormFile("site_zip")
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "site_zip file is required")
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		utils.WriteError(w, http.StatusBadRequest, "site_zip must be a .zip file")
		return
	}

	if err := os.MkdirAll("uploads", 0755); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	localZipPath := filepath.Join("uploads", hostName+".zip")
	dst, err := os.Create(localZipPath)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer dst.Close()
	if _, err := dst.ReadFrom(file); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	allocateMu.Lock()
	instances, err := storage.LoadInstances()
	if err != nil {
		allocateMu.Unlock()
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, ins := range instances {
		if ins.HostName == hostName {
			allocateMu.Unlock()
			utils.WriteError(w, http.StatusConflict, "host already exists")
			return
		}
	}
	staticIP, err := allocateNextIP()
	if err != nil {
		allocateMu.Unlock()
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	placeholder := models.WebInstance{
		HostName:  hostName,
		IP:        staticIP,
		VMName:    vmName,
		CreatedAt: time.Now().Format(time.RFC3339),
		Status:    "creating",
	}
	saved, err := storage.AddInstance(placeholder)
	allocateMu.Unlock()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go func(ins models.WebInstance, zipPath string) {
		if err := provisionInstance(ins, zipPath); err != nil {
			fmt.Printf("provision %s failed: %v\n", ins.HostName, err)
			storage.UpdateInstanceStatus(ins.HostName, "error")
			return
		}
		storage.UpdateInstanceStatus(ins.HostName, "running")
	}(saved, localZipPath)

	utils.WriteJSON(w, http.StatusAccepted, saved)
}

// provisionInstance runs the full create pipeline: clone-equivalent VM creation
// from the multi-attach base, network reconfiguration, DNS registration, and
// site deployment. Holds provisionMu only across the steps that depend on
// Cfg.TemplateInitialIP being unique.
func provisionInstance(ins models.WebInstance, zipPath string) error {
	provisionMu.Lock()
	if err := services.CreateApacheVM(services.VMConfig{
		VMName:      ins.VMName,
		BaseVDIPath: Cfg.BaseVDIPath,
		HostOnlyIF:  Cfg.HostOnlyIF,
		MemoryMB:    Cfg.VMMemoryMB,
		CPUs:        Cfg.VMCPUs,
		OSType:      Cfg.VMOSType,
	}); err != nil {
		provisionMu.Unlock()
		return err
	}

	if err := waitForSSH(Cfg.TemplateInitialIP, Cfg.SSHUser); err != nil {
		provisionMu.Unlock()
		return err
	}

	if err := services.ConfigureClone(
		Cfg.TemplateInitialIP, Cfg.SSHUser, ins.HostName, ins.IP, Cfg.IPPrefix,
	); err != nil {
		provisionMu.Unlock()
		return err
	}

	if err := waitForSSH(ins.IP, Cfg.SSHUser); err != nil {
		provisionMu.Unlock()
		return err
	}
	provisionMu.Unlock()

	if err := services.AddDNSRecord(services.DNSConfig{
		Host: Cfg.DNSAuthoritativeIP,
		User: Cfg.DNSSSHUser,
		Zone: Cfg.Domain,
	}, ins.HostName, ins.IP); err != nil {
		return err
	}

	if err := services.DeployZipToApache(ins.IP, Cfg.SSHUser, zipPath); err != nil {
		return err
	}

	if err := os.Remove(zipPath); err != nil && !os.IsNotExist(err) {
		fmt.Printf("warning: could not remove %s: %v\n", zipPath, err)
	}

	return nil
}

// DeleteInstance marks the record as "deleting", returns 202, and runs the
// VM/DNS/store teardown in a goroutine. The instance disappears from the table
// only after the cleanup completes.
func DeleteInstance(w http.ResponseWriter, r *http.Request) {
	hostName := strings.TrimSpace(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}
	hostName = sanitizeHostName(hostName)

	target, found, err := findInstance(hostName)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		utils.WriteError(w, http.StatusNotFound, "instance not found")
		return
	}

	if _, _, err := storage.UpdateInstanceStatus(hostName, "deleting"); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go func(ins models.WebInstance) {
		if err := services.UnregisterVM(ins.VMName); err != nil {
			fmt.Printf("unregister %s failed: %v\n", ins.VMName, err)
		}
		if err := services.RemoveDNSRecord(services.DNSConfig{
			Host: Cfg.DNSAuthoritativeIP,
			User: Cfg.DNSSSHUser,
			Zone: Cfg.Domain,
		}, ins.HostName); err != nil {
			fmt.Printf("remove dns %s failed: %v\n", ins.HostName, err)
		}
		if _, _, err := storage.RemoveInstanceByHostName(ins.HostName); err != nil {
			fmt.Printf("remove store %s failed: %v\n", ins.HostName, err)
		}
	}(target)

	utils.WriteJSON(w, http.StatusAccepted, map[string]string{
		"deleting": hostName,
	})
}

// StartInstance powers on the VM, marks "starting", and only flips to
// "running" once SSH is reachable on the static IP.
func StartInstance(w http.ResponseWriter, r *http.Request) {
	hostName := strings.TrimSpace(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}
	hostName = sanitizeHostName(hostName)

	target, found, err := findInstance(hostName)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		utils.WriteError(w, http.StatusNotFound, "instance not found")
		return
	}

	if err := services.StartVM(target.VMName); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, _, err := storage.UpdateInstanceStatus(hostName, "starting")
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go func(ip, host string) {
		if err := waitForSSH(ip, Cfg.SSHUser); err != nil {
			storage.UpdateInstanceStatus(host, "stopped")
			return
		}
		storage.UpdateInstanceStatus(host, "running")
	}(target.IP, hostName)

	utils.WriteJSON(w, http.StatusAccepted, updated)
}

// StopInstance sends ACPI shutdown, waits up to ~30 s for the guest to power
// itself off, then escalates to a forced VBox poweroff so the VM is reliably
// off when the UI says "stopped".
func StopInstance(w http.ResponseWriter, r *http.Request) {
	hostName := strings.TrimSpace(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}
	hostName = sanitizeHostName(hostName)

	target, found, err := findInstance(hostName)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		utils.WriteError(w, http.StatusNotFound, "instance not found")
		return
	}

	if err := services.StopVM(target.VMName); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, _, err := storage.UpdateInstanceStatus(hostName, "stopping")
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go func(vm, host string) {
		powerDownAndConfirm(vm)
		storage.UpdateInstanceStatus(host, "stopped")
	}(target.VMName, hostName)

	utils.WriteJSON(w, http.StatusAccepted, updated)
}

// powerDownAndConfirm waits up to 30 s for ACPI shutdown to complete, then
// forces a hard poweroff if the guest didn't honor the ACPI event (common when
// acpid is missing in the guest). Returns when VBox reports the VM as off.
func powerDownAndConfirm(vmName string) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if state, _ := services.VMState(vmName); state == "poweroff" {
			return
		}
		time.Sleep(2 * time.Second)
	}
	if err := services.ForcePowerOffVM(vmName); err != nil {
		fmt.Printf("force poweroff %s: %v\n", vmName, err)
	}
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Second)
		if state, _ := services.VMState(vmName); state == "poweroff" {
			return
		}
	}
}

func findInstance(hostName string) (models.WebInstance, bool, error) {
	instances, err := storage.LoadInstances()
	if err != nil {
		return models.WebInstance{}, false, err
	}
	for _, ins := range instances {
		if ins.HostName == hostName {
			return ins, true, nil
		}
	}
	return models.WebInstance{}, false, nil
}

func waitForSSH(ip string, user string) error {
	cfg := services.SSHConfig{User: user, Host: ip}
	for i := 0; i < 30; i++ {
		_, err := services.RunSSHCommand(cfg, "echo ok")
		if err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("ssh not available on %s", ip)
}

func allocateNextIP() (string, error) {
	instances, err := storage.LoadInstances()
	if err != nil {
		return "", err
	}
	used := make(map[string]bool)
	for _, ins := range instances {
		used[ins.IP] = true
	}
	for octet := Cfg.IPStart; octet < 255; octet++ {
		candidate := fmt.Sprintf("%s%d", Cfg.IPPrefix, octet)
		if !used[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free IP in %s0/24", Cfg.IPPrefix)
}

func sanitizeHostName(input string) string {
	input = strings.ToLower(input)
	input = strings.ReplaceAll(input, " ", "-")
	input = strings.ReplaceAll(input, "_", "-")
	clean := make([]rune, 0, len(input))
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			clean = append(clean, r)
		}
	}
	return string(clean)
}
