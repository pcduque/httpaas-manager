package services

import (
	"fmt"
)

type DNSConfig struct {
	Host       string
	User       string
	Zone       string
	ZoneFile   string
	RemotePath string
}

func AddDNSRecord(cfg DNSConfig, hostName string, ipAddress string) error {
	if cfg.User == "" {
		cfg.User = "server1"
	}

	if cfg.Zone == "" {
		cfg.Zone = "cloud.local"
	}

	if cfg.ZoneFile == "" {
		cfg.ZoneFile = "/etc/bind/db.cloud.local"
	}

	sshCfg := SSHConfig{
		User: cfg.User,
		Host: cfg.Host,
	}

	record := fmt.Sprintf("%s    IN    A    %s", hostName, ipAddress)

	cmd := fmt.Sprintf(`
if ! grep -q "^%s[[:space:]]" %s; then
  echo "%s" | sudo tee -a %s
fi
sudo named-checkzone %s %s &&
sudo systemctl restart bind9
`, hostName, cfg.ZoneFile, record, cfg.ZoneFile, cfg.Zone, cfg.ZoneFile)

	if output, err := RunSSHCommand(sshCfg, cmd); err != nil {
		return fmt.Errorf("error adding DNS record: %v - output: %s", err, output)
	}

	return nil
}