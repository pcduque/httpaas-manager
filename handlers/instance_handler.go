package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"httpaas-manager/models"
	"httpaas-manager/services"
	"httpaas-manager/storage"
	"httpaas-manager/utils"
)

const (
	DefaultDomain       = "cloud.local"
	DefaultTemplateVM   = "ApacheServer"
	DefaultSSHUser      = "server1"
	DefaultDNSServerIP  = "192.168.10.10"
	DefaultBridgeAdapter = "Intel(R) Dual Band Wireless-AC 8265"
)

func ListInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := storage.LoadInstances()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, instances)
}

func CreateInstance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	hostName := strings.TrimSpace(r.FormValue("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}

	hostName = sanitizeHostName(hostName)
	vmName := hostName
	fqdn := fmt.Sprintf("%s.%s", hostName, DefaultDomain)

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

	vmCfg := services.VMConfig{
		VMName:            vmName,
		TemplateVMName:    DefaultTemplateVM,
		BridgeAdapterName: DefaultBridgeAdapter,
	}

	if err := services.CreateApacheVM(vmCfg); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ip, err := services.GetVMIPAddress(vmName)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := waitForSSH(ip, DefaultSSHUser); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := services.ConfigureHostname(ip, DefaultSSHUser, hostName); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := services.DeployZipToApache(ip, DefaultSSHUser, localZipPath); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	dnsCfg := services.DNSConfig{
		Host:     DefaultDNSServerIP,
		User:     DefaultSSHUser,
		Zone:     DefaultDomain,
		ZoneFile: "/etc/bind/db.cloud.local",
	}

	if err := services.AddDNSRecord(dnsCfg, hostName, ip); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	instance := models.WebInstance{
		HostName:  hostName,
		Domain:    DefaultDomain,
		FQDN:      fqdn,
		IPAddress: ip,
		VMName:    vmName,
		Status:    "running",
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	saved, err := storage.AddInstance(instance)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusCreated, saved)
}

func waitForSSH(ip string, user string) error {
	cfg := services.SSHConfig{
		User: user,
		Host: ip,
	}

	for i := 0; i < 20; i++ {
		_, err := services.RunSSHCommand(cfg, "echo ok")
		if err == nil {
			return nil
		}

		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("ssh not available on %s", ip)
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