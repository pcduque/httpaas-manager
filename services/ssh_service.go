package services

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
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
	// KeyPath is the optional path to a PEM-encoded SSH private key. When set
	// and loadable, key auth is tried before password auth so guest VMs that
	// already have the matching public key in authorized_keys log in without
	// a password prompt. Empty falls back to password-only.
	KeyPath string
}

// guestKeyPath is the manager-wide default for the guest SSH key. Set by
// InitGuestSSHKey at startup so per-call SSHConfig values don't need to carry
// the path; callers that want a different key still override it on SSHConfig.
var guestKeyPath string

// InitGuestSSHKey records the default private key path used for guest SSH
// auth. Safe to call multiple times; later calls overwrite. Empty string
// disables key auth and falls back to password.
func InitGuestSSHKey(path string) {
	guestKeyPath = path
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

	keyPath := cfg.KeyPath
	if keyPath == "" {
		keyPath = guestKeyPath
	}

	auths := make([]ssh.AuthMethod, 0, 2)
	if keyPath != "" {
		if signer, err := loadSSHKey(keyPath); err == nil {
			auths = append(auths, ssh.PublicKeys(signer))
		}
	}
	auths = append(auths, ssh.Password(pass))

	clientCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(cfg.Host, "22")
	return ssh.Dial("tcp", addr, clientCfg)
}

// loadSSHKey reads a PEM-encoded private key from disk and parses it into an
// ssh.Signer. Returns an error if the file is missing, unreadable, or not a
// valid key — callers downgrade to password auth in that case.
func loadSSHKey(path string) (ssh.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(data)
}

// ReadAuthorizedKey returns the OpenSSH-format public key matching the given
// private key path (i.e. <path>.pub). Used at template prep time to bake the
// manager's key into the guest's authorized_keys. Returns an error if the
// .pub file is missing.
func ReadAuthorizedKey(privateKeyPath string) (string, error) {
	pubPath := privateKeyPath + ".pub"
	data, err := os.ReadFile(pubPath)
	if err != nil {
		return "", fmt.Errorf("read public key %s: %w", pubPath, err)
	}
	return strings.TrimSpace(string(data)) + "\n", nil
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
//
// NOTE: in non-TTY SSH sessions sudo's credential cache is unreliable across
// successive sudo invocations — we've seen the 4th sudo lose the cache and
// fail with "a terminal is required". For new code prefer SudoBashWrap, which
// runs the whole privileged block under a single sudo invocation.
func SudoPrelude() string {
	return fmt.Sprintf(`echo '%s' | sudo -S -v 2>/dev/null
`, config.SSHPassword)
}

// SudoBashWrap runs `body` as root in a single `sudo -S bash -c` invocation,
// piping the password on stdin. Use when a script needs root for several
// commands in a row; avoids relying on sudo's cache which doesn't persist
// reliably across non-TTY SSH sessions. The body is single-quoted, so any
// literal single quotes inside it must be escaped by the caller (or written
// in $'..' style).
func SudoBashWrap(body string) string {
	return fmt.Sprintf(`echo '%s' | sudo -S bash -c '
set -e
%s
'`, config.SSHPassword, body)
}

// SudoRunScript stages `body` to a per-PID file in /tmp (writable by the SSH
// user without sudo), then runs it under a single sudo bash invocation, and
// finally removes it. Use this when the script is too complex for
// SudoBashWrap (single quotes, nested heredocs) — the heredoc-quoted delimiter
// here lets the body contain arbitrary characters except its own delimiter.
//
// Inside `body` the script is already root, so do NOT add sudo to its
// commands. Doing so triggers the cache problem that motivated this helper.
func SudoRunScript(body string) string {
	return fmt.Sprintf(`set -e
SCRIPT_PATH="/tmp/dbaas-run-$$.sh"
cat > "$SCRIPT_PATH" <<'DBAAS_SCRIPT_EOF'
%s
DBAAS_SCRIPT_EOF
echo '%s' | sudo -S bash "$SCRIPT_PATH"
rm -f "$SCRIPT_PATH"
`, body, config.SSHPassword)
}
