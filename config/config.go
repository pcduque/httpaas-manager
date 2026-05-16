package config

import (
	"os"
	"runtime"
	"strconv"
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

	BaseVDIPath string
	VMMemoryMB  int
	VMCPUs      int
	VMOSType    string

	MariaDBBaseVDIPath  string
	PostgresBaseVDIPath string
	MariaDBPort         int
	PostgresPort        int
	DefaultDBUser       string
	DefaultDBPassword   string

	// DBRootPassword is the password baked into both DB templates for the
	// engine's superuser (root@'%' for MariaDB, postgres role for PostgreSQL)
	// so administrators can connect from anywhere on the host-only subnet.
	DBRootPassword string

	// GuestSSHKeyPath is the SSH private key the manager uses to log into
	// guest VMs (template prep + provisioned instances). The matching public
	// key is baked into /home/<SSHUser>/.ssh/authorized_keys at template prep
	// time. Falls back to password auth if the key isn't loadable.
	GuestSSHKeyPath string
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

		BaseVDIPath: getEnv("BASE_VDI_PATH", `C:\Users\Orlay Molina\VirtualBox VMs\debian-web1.vdi`),
		VMMemoryMB:  getInt("VM_MEMORY_MB", 1024),
		VMCPUs:      getInt("VM_CPUS", 1),
		VMOSType:    getEnv("VM_OSTYPE", "Debian_64"),

		MariaDBBaseVDIPath:  getEnv("MARIADB_BASE_VDI", `C:\Users\Orlay Molina\VirtualBox VMs\debian-mariadb.vdi`),
		PostgresBaseVDIPath: getEnv("POSTGRES_BASE_VDI", `C:\Users\Orlay Molina\VirtualBox VMs\debian-postgres.vdi`),
		MariaDBPort:         getInt("MARIADB_PORT", 3306),
		PostgresPort:        getInt("POSTGRES_PORT", 5432),
		DefaultDBUser:       getEnv("DB_DEFAULT_USER", "admin"),
		DefaultDBPassword:   getEnv("DB_DEFAULT_PASSWORD", "admin123"),
		DBRootPassword:      getEnv("DB_ROOT_PASSWORD", "root123"),
		GuestSSHKeyPath:     getEnv("GUEST_SSH_KEY", expandHome("~/.ssh/id_ed25519")),
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

func getInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// expandHome replaces a leading "~/" with the current user's home directory.
// Used for default config values that reference SSH keys.
func expandHome(p string) string {
	if !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return home + p[1:]
}
