package services

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"httpaas-manager/config"
)

type SSHConfig struct {
	User       string
	Host       string
	Password   string
	RemotePath string
}

func sshClient(cfg SSHConfig) (*ssh.Client, error) {
	user := cfg.User
	if user == "" {
		user = config.SSHUser
	}
	pass := cfg.Password
	if pass == "" {
		pass = config.SSHPassword
	}

	clientCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(cfg.Host, "22")
	return ssh.Dial("tcp", addr, clientCfg)
}

// RunSSHCommand executes a remote shell command and returns combined stdout+stderr.
func RunSSHCommand(cfg SSHConfig, command string) (string, error) {
	client, err := sshClient(cfg)
	if err != nil {
		return "", fmt.Errorf("ssh dial %s: %w", cfg.Host, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = &out
	if err := session.Run(command); err != nil {
		return out.String(), fmt.Errorf("ssh run: %w", err)
	}
	return out.String(), nil
}

// CopyFileBySCP uploads a local file to cfg.RemotePath via SFTP (works over the
// same SSH transport, so the password from cfg is reused).
func CopyFileBySCP(cfg SSHConfig, localPath string) (string, error) {
	client, err := sshClient(cfg)
	if err != nil {
		return "", fmt.Errorf("ssh dial %s: %w", cfg.Host, err)
	}
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("sftp open: %w", err)
	}
	defer sftpClient.Close()

	src, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := sftpClient.Create(cfg.RemotePath)
	if err != nil {
		return "", fmt.Errorf("sftp create %s: %w", cfg.RemotePath, err)
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return "", fmt.Errorf("sftp copy: %w", err)
	}
	return fmt.Sprintf("uploaded %d bytes to %s", n, cfg.RemotePath), nil
}

// SudoPrelude returns a shell snippet that primes sudo using SSH_PASSWORD so
// subsequent sudo calls in the same session don't prompt. Insert at the top
// of any script that uses sudo.
func SudoPrelude() string {
	return fmt.Sprintf(`echo '%s' | sudo -S -v 2>/dev/null
`, config.SSHPassword)
}
