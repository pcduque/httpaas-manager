package config

import (
	"os"
	"runtime"
	"strings"
)

var (
	SSHUser     = getEnv("SSH_USER", "orlay")
	SSHPassword = getEnv("SSH_PASSWORD", "1234567")
	DNSServerIP = getEnv("DNS_SERVER_IP", "192.168.10.10")
	Domain      = getEnv("DOMAIN", "cloud.local")
	TemplateVM  = "debian-web1"
	HostOnlyIF  = "VirtualBox Host-Only Ethernet Adapter"
	IPPrefix    = "192.168.10."
	IPStart     = 30
	AppPort     = "8080"
)

type Config struct {
	HTTPPort string

	VBoxLocal      bool
	WindowsHost    string
	WindowsUser    string
	SSHKeyPath     string
	VBoxManagePath string

	TemplateVMName string
	HostOnlyIF     string

	SSHUser     string
	SSHPassword string

	Domain             string
	DNSAuthoritativeIP string
	DNSSSHUser         string

	IPPrefix string
	IPStart  int

	TemplateInitialIP string
}

func Load() Config {
	return Config{
		HTTPPort: getEnv("HTTPAAS_PORT", AppPort),

		VBoxLocal:      getBool("VBOX_LOCAL", runtime.GOOS == "windows"),
		WindowsHost:    getEnv("WIN_HOST", "192.168.10.1"),
		WindowsUser:    getEnv("WIN_USER", "Orlay Molina"),
		SSHKeyPath:     getEnv("WIN_SSH_KEY", "/home/orlay/.ssh/id_ed25519"),
		VBoxManagePath: getEnv("VBOX_PATH", `C:\Program Files\Oracle\VirtualBox\VBoxManage.exe`),

		TemplateVMName: getEnv("TEMPLATE_VM", TemplateVM),
		HostOnlyIF:     getEnv("HOSTONLY_IF", HostOnlyIF),

		SSHUser:     getEnv("GUEST_SSH_USER", SSHUser),
		SSHPassword: getEnv("GUEST_SSH_PASSWORD", SSHPassword),

		Domain:             getEnv("DOMAIN", Domain),
		DNSAuthoritativeIP: getEnv("DNS_AUTH_IP", DNSServerIP),
		DNSSSHUser:         getEnv("DNS_SSH_USER", SSHUser),

		IPPrefix: getEnv("IP_PREFIX", IPPrefix),
		IPStart:  IPStart,

		TemplateInitialIP: getEnv("TEMPLATE_INITIAL_IP", "192.168.10.20"),
	}
}

func getEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	}
	return def
}
