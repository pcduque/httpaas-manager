package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DBeaverConnection is the minimum spec needed to register one entry in
// DBeaver via its CLI. Driver names follow DBeaver's internal IDs
// (e.g. "postgres-jdbc" for PostgreSQL, "mariadb" for MariaDB).
type DBeaverConnection struct {
	Name     string
	Driver   string
	Host     string
	Port     int
	Database string
	User     string
	Password string
	Folder   string
}

// RegisterDBeaverConnection invokes dbeaver-cli.exe with -con "<spec>" so the
// connection is created (and the password saved) in the user's DBeaver
// workspace. The CLI either spins up DBeaver or hands the request to an
// already-running instance; in both cases the entry persists across restarts
// because savePassword=true and create=true are part of the spec.
//
// We invoke the CLI rather than editing data-sources.json directly to dodge
// DBeaver's credential encryption (AES with derived keys; format has shifted
// between versions, easy to corrupt the workspace).
func RegisterDBeaverConnection(conn DBeaverConnection) error {
	if conn.Driver == "" {
		return fmt.Errorf("driver is required")
	}
	if conn.Host == "" || conn.Port == 0 {
		return fmt.Errorf("host and port are required")
	}

	cli, err := resolveDBeaverCLI()
	if err != nil {
		return err
	}

	spec := buildDBeaverConnSpec(conn)
	cmd := exec.Command(cli, "-con", spec)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dbeaver-cli -con failed: %v - %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resolveDBeaverCLI prefers an explicit DBEAVER_CLI_PATH and otherwise probes
// the per-user and machine-wide install locations on Windows. DBeaver CE on
// Windows ships `dbeaver.exe` (GUI launcher) and `dbeaverc.exe` (console
// variant); both accept `-con`. We probe `dbeaver.exe` first because it's
// always present, falling back to the EE-style `dbeaver-cli.exe` for users on
// Eclipse-flavored installs.
func resolveDBeaverCLI() (string, error) {
	if p := strings.TrimSpace(os.Getenv("DBEAVER_CLI_PATH")); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("DBEAVER_CLI_PATH=%s not found on disk", p)
	}

	roots := []string{
		`C:\Program Files\DBeaver`,
		`C:\Program Files (x86)\DBeaver`,
	}
	if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
		roots = append(roots,
			filepath.Join(local, "DBeaver"),
			filepath.Join(local, "Programs", "DBeaver"),
		)
	}
	binaries := []string{"dbeaver.exe", "dbeaverc.exe", "dbeaver-cli.exe"}
	for _, root := range roots {
		for _, bin := range binaries {
			candidate := filepath.Join(root, bin)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("DBeaver executable not found in standard locations; set DBEAVER_CLI_PATH to the full path of dbeaver.exe")
}

// buildDBeaverConnSpec assembles the pipe-separated key=value string that
// DBeaver's -con flag expects. `create=true` ensures the connection is added
// to the workspace; `savePassword=true` writes the credential into DBeaver's
// own encrypted credentials store; `connect=false`/`openConsole=false` avoid
// stealing focus by auto-opening the SQL editor.
func buildDBeaverConnSpec(c DBeaverConnection) string {
	parts := []string{
		"driver=" + c.Driver,
		"name=" + sanitizeDBeaverField(c.Name),
		"host=" + c.Host,
		"port=" + fmt.Sprintf("%d", c.Port),
		"database=" + sanitizeDBeaverField(c.Database),
		"user=" + sanitizeDBeaverField(c.User),
		"password=" + sanitizeDBeaverField(c.Password),
		"savePassword=true",
		"create=true",
		"connect=false",
		"openConsole=false",
	}
	if c.Folder != "" {
		parts = append(parts, "folder="+sanitizeDBeaverField(c.Folder))
	}
	return strings.Join(parts, "|")
}

// sanitizeDBeaverField strips the pipe character (the spec's field separator)
// so a stray '|' inside a value cannot break parsing. The DB password charset
// already excludes '|', so this is mostly defense in depth.
func sanitizeDBeaverField(s string) string {
	return strings.ReplaceAll(s, "|", "")
}
