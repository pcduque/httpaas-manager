package services

import (
	"fmt"
	"strings"
)

type DNSConfig struct {
	Host string
	User string
	Zone string
}

// AddDNSRecord adds an A record for hostName.<zone> pointing to ipAddress.
// Executes nsupdate on the authoritative DNS server (ns1) over SSH so the
// TSIG key stored there is used. FQDNs always include the trailing dot.
func AddDNSRecord(cfg DNSConfig, hostName string, ipAddress string) error {
	zone := strings.TrimSuffix(cfg.Zone, ".")
	fqdn := fmt.Sprintf("%s.%s.", hostName, zone)

	script := fmt.Sprintf(`nsupdate -k /etc/bind/rndc.key <<EOF
server 127.0.0.1
zone %s.
update delete %s A
update add %s 300 A %s
send
EOF`, zone, fqdn, fqdn, ipAddress)

	sshCfg := SSHConfig{User: cfg.User, Host: cfg.Host}
	if output, err := RunSSHCommand(sshCfg, script); err != nil {
		return fmt.Errorf("nsupdate add %s: %v - output: %s", fqdn, err, output)
	}
	return nil
}

// RemoveDNSRecord deletes the A record for hostName.<zone>.
func RemoveDNSRecord(cfg DNSConfig, hostName string) error {
	zone := strings.TrimSuffix(cfg.Zone, ".")
	fqdn := fmt.Sprintf("%s.%s.", hostName, zone)

	script := fmt.Sprintf(`nsupdate -k /etc/bind/rndc.key <<EOF
server 127.0.0.1
zone %s.
update delete %s A
send
EOF`, zone, fqdn)

	sshCfg := SSHConfig{User: cfg.User, Host: cfg.Host}
	if output, err := RunSSHCommand(sshCfg, script); err != nil {
		return fmt.Errorf("nsupdate remove %s: %v - output: %s", fqdn, err, output)
	}
	return nil
}
