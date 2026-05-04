package services

import (
	"fmt"
	"path"
	"strings"

	"httpaas-manager/config"
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
	cmd := fmt.Sprintf(`exec 2>&1
echo "::TRACE_v3:: ssh start on $(hostname) as $(whoami) target=%s"
ls -la %s || echo "::TRACE:: remote zip not visible to %s"
echo '%s' | sudo -S bash -c '
echo "::TRACE:: inside sudo as $(whoami)"
LOG=/tmp/httpaas-deploy.log
(
  set -ex
  command -v unzip >/dev/null 2>&1 || { echo "ERROR: unzip is not installed on the target VM"; exit 1; }
  test -s %s || { echo "ERROR: uploaded zip is missing or empty"; exit 1; }
  mkdir -p /var/www/html
  rm -rf /var/www/html/*
  unzip -o %s -d /var/www/html
  chown -R www-data:www-data /var/www/html
  systemctl reload apache2 || systemctl restart apache2
  rm -f %s
) >"$LOG" 2>&1
status=$?
echo "::DEPLOY_LOG_BEGIN (exit $status)::"
cat "$LOG" 2>&1 || echo "(log file not readable)"
echo "::DEPLOY_LOG_END::"
exit $status
'
SUDO_STATUS=$?
echo "::TRACE:: sudo exited with $SUDO_STATUS"
exit $SUDO_STATUS
`, remoteZip, remoteZip, sshUser, config.SSHPassword, remoteZip, remoteZip, remoteZip)

	if output, err := RunSSHCommand(sshCfg, cmd); err != nil {
		return fmt.Errorf("deploy zip on %s: %v - output: %s", targetIP, err, output)
	}
	return nil
}
