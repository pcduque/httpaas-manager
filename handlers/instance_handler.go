package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"httpaas-manager/config"
	"httpaas-manager/models"
	"httpaas-manager/services"
	"httpaas-manager/storage"
	"httpaas-manager/utils"
)

var Cfg config.Config

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

	staticIP, err := allocateNextIP()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := services.CloneApacheVM(services.VMConfig{
		VMName:         vmName,
		TemplateVMName: Cfg.TemplateVMName,
		HostOnlyIF:     Cfg.HostOnlyIF,
	}); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := waitForSSH(Cfg.TemplateInitialIP, Cfg.SSHUser); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := services.ConfigureClone(
		Cfg.TemplateInitialIP, Cfg.SSHUser, hostName, staticIP, Cfg.IPPrefix,
	); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := waitForSSH(staticIP, Cfg.SSHUser); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := services.AddDNSRecord(services.DNSConfig{
		Host: Cfg.DNSAuthoritativeIP,
		User: Cfg.DNSSSHUser,
		Zone: Cfg.Domain,
	}, hostName, staticIP); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := services.DeployZipToApache(staticIP, Cfg.SSHUser, localZipPath); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	instance := models.WebInstance{
		HostName:  hostName,
		IP:        staticIP,
		VMName:    vmName,
		CreatedAt: time.Now().Format(time.RFC3339),
		Status:    "running",
	}
	saved, err := storage.AddInstance(instance)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.WriteJSON(w, http.StatusCreated, saved)
}

func DeleteInstance(w http.ResponseWriter, r *http.Request) {
	hostName := strings.TrimSpace(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}
	hostName = sanitizeHostName(hostName)

	removed, found, err := storage.RemoveInstanceByHostName(hostName)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		utils.WriteError(w, http.StatusNotFound, "instance not found")
		return
	}

	if err := services.UnregisterVM(removed.VMName); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := services.RemoveDNSRecord(services.DNSConfig{
		Host: Cfg.DNSAuthoritativeIP,
		User: Cfg.DNSSSHUser,
		Zone: Cfg.Domain,
	}, hostName); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"deleted": hostName,
		"vm":      removed.VMName,
		"ip":      removed.IP,
	})
}

func StartInstance(w http.ResponseWriter, r *http.Request) {
	hostName := strings.TrimSpace(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}
	hostName = sanitizeHostName(hostName)

	instances, err := storage.LoadInstances()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var target models.WebInstance
	found := false
	for _, ins := range instances {
		if ins.HostName == hostName {
			target = ins
			found = true
			break
		}
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

func StopInstance(w http.ResponseWriter, r *http.Request) {
	hostName := strings.TrimSpace(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}
	hostName = sanitizeHostName(hostName)

	instances, err := storage.LoadInstances()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var target models.WebInstance
	found := false
	for _, ins := range instances {
		if ins.HostName == hostName {
			target = ins
			found = true
			break
		}
	}
	if !found {
		utils.WriteError(w, http.StatusNotFound, "instance not found")
		return
	}

	if err := services.StopVM(target.VMName); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, _, err := storage.UpdateInstanceStatus(hostName, "stopped")
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.WriteJSON(w, http.StatusOK, updated)
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
