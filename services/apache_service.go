package services

import (
	"fmt"
	"path"
	"strings"
)

// ConfigureClone sets the hostname and applies the static IP on the freshly
// cloned VM without restarting networking. The new IP is added live with
// `ip addr add` so the clone is reachable on it immediately while keeping the
// initialIP bound (so this very SSH session doesn't get killed). The persistent
// config is written for future reboots, and the old IP is dropped in a detached
// subshell after the SSH session returns.
func ConfigureClone(initialIP, sshUser, hostName, staticIP, prefix string) error {
	_ = prefix
	cfg := SSHConfig{User: sshUser, Host: initialIP}

	cmd := SudoPrelude() + fmt.Sprintf(`set -e
sudo ip addr add %s/24 dev enp0s3 2>/dev/null || true

echo '%s' | sudo tee /etc/hostname >/dev/null
sudo hostnamectl set-hostname %s

if grep -qE '^127\.0\.1\.1' /etc/hosts; then
    sudo sed -i "s/^127\.0\.1\.1.*/127.0.1.1 %s/" /etc/hosts
else
    echo "127.0.1.1 %s" | sudo tee -a /etc/hosts >/dev/null
fi

sudo tee /etc/network/interfaces >/dev/null <<NETEOF
auto lo
iface lo inet loopback

auto enp0s3
iface enp0s3 inet static
    address %s
    netmask 255.255.255.0
NETEOF

(sleep 5 && sudo ip addr del %s/24 dev enp0s3 2>/dev/null) &
disown 2>/dev/null || true
exit 0
`, staticIP, hostName, hostName, hostName, hostName, staticIP, initialIP)

	if output, err := RunSSHCommand(cfg, cmd); err != nil {
		return fmt.Errorf("configure clone (hostname/IP): %v - output: %s", err, output)
	}
	return nil
}

// DeployZipToApache uploads the .zip to the target VM, unzips it under
// /var/www/html and reloads Apache. Apache is assumed already installed in the
// template VM, so no apt install is performed.
func DeployZipToApache(targetIP, sshUser, localZipPath string) error {
	remoteFileName := path.Base(strings.ReplaceAll(localZipPath, "\\", "/"))
	remoteZip := path.Join("/home", sshUser, remoteFileName)

	scpCfg := SSHConfig{
		User:       sshUser,
		Host:       targetIP,
		RemotePath: remoteZip,
	}
	if output, err := CopyFileBySCP(scpCfg, localZipPath); err != nil {
		return fmt.Errorf("scp zip to %s: %v - output: %s", targetIP, err, output)
	}

	sshCfg := SSHConfig{User: sshUser, Host: targetIP}
	cmd := SudoPrelude() + fmt.Sprintf(`set -e
sudo rm -rf /var/www/html/*
sudo unzip -o %s -d /var/www/html
sudo chown -R www-data:www-data /var/www/html
sudo systemctl reload apache2 || sudo systemctl restart apache2
rm -f %s
`, remoteZip, remoteZip)

	if output, err := RunSSHCommand(sshCfg, cmd); err != nil {
		return fmt.Errorf("deploy zip on %s: %v - output: %s", targetIP, err, output)
	}
	return nil
}
