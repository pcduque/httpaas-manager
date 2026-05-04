package services

import (
	"fmt"
	"path"
	"strings"
)

// ConfigureClone sets the hostname and applies the static IP on the freshly
// cloned VM. Networking is restarted in a detached subshell so the SSH session
// returns cleanly before the interface flap kicks the connection.
func ConfigureClone(initialIP, sshUser, hostName, staticIP, prefix string) error {
	cfg := SSHConfig{User: sshUser, Host: initialIP}

	cmd := fmt.Sprintf(`set -e
echo '%s' | sudo tee /etc/hostname >/dev/null
sudo hostnamectl set-hostname %s

sudo tee /etc/network/interfaces >/dev/null <<NETEOF
auto lo
iface lo inet loopback

auto enp0s3
iface enp0s3 inet static
    address %s
    netmask 255.255.255.0
NETEOF

(sleep 2 && sudo systemctl restart networking) &
exit 0
`, hostName, hostName, staticIP)

	_ = prefix
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
	cmd := fmt.Sprintf(`set -e
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
