package services

import (
	"fmt"
)

func DeployZipToApache(ipAddress string, sshUser string, zipPath string) error {
	if sshUser == "" {
		sshUser = "server1"
	}

	cfg := SSHConfig{
		User:       sshUser,
		Host:       ipAddress,
		RemotePath: "/home/" + sshUser,
	}

	if output, err := CopyFileBySCP(cfg, zipPath); err != nil {
		return fmt.Errorf("error copying zip to apache VM: %v - output: %s", err, output)
	}

	remoteZip := fmt.Sprintf("/home/%s/site.zip", sshUser)

	cmd := fmt.Sprintf(`
sudo apt update -y &&
sudo apt install apache2 unzip -y &&
sudo rm -rf /var/www/html/* &&
sudo mv /home/%s/%s %s &&
sudo unzip -o %s -d /var/www/html &&
sudo chown -R www-data:www-data /var/www/html &&
sudo systemctl enable apache2 &&
sudo systemctl restart apache2
`, sshUser, getFileName(zipPath), remoteZip, remoteZip)

	if output, err := RunSSHCommand(cfg, cmd); err != nil {
		return fmt.Errorf("error deploying apache site: %v - output: %s", err, output)
	}

	return nil
}

func ConfigureHostname(ipAddress string, sshUser string, hostName string) error {
	if sshUser == "" {
		sshUser = "server1"
	}

	cfg := SSHConfig{
		User: sshUser,
		Host: ipAddress,
	}

	cmd := fmt.Sprintf(`
echo '%s' | sudo tee /etc/hostname &&
sudo hostnamectl set-hostname %s
`, hostName, hostName)

	if output, err := RunSSHCommand(cfg, cmd); err != nil {
		return fmt.Errorf("error configuring hostname: %v - output: %s", err, output)
	}

	return nil
}

func getFileName(path string) string {
	lastSlash := -1
	for i, ch := range path {
		if ch == '/' || ch == '\\' {
			lastSlash = i
		}
	}

	if lastSlash >= 0 && lastSlash+1 < len(path) {
		return path[lastSlash+1:]
	}

	return path
}