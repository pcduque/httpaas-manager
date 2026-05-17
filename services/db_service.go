package services

import (
	"fmt"
	"path"
	"strings"
)

// EngineMariaDB and EnginePostgres are the canonical engine identifiers used
// across handlers, storage, and the dashboard.
const (
	EngineMariaDB  = "mariadb"
	EnginePostgres = "postgres"
)

// EngineSubdomain returns the DNS subdomain used for instances of an engine,
// so FQDNs become <name>.mdb.<domain> or <name>.pg.<domain>. Keeping the
// mapping centralized avoids drift between handlers and the UI.
func EngineSubdomain(engine string) string {
	switch strings.ToLower(engine) {
	case EngineMariaDB:
		return "mdb"
	case EnginePostgres:
		return "pg"
	default:
		return ""
	}
}

// EngineDefaultPort returns 3306 for MariaDB and 5432 for PostgreSQL.
func EngineDefaultPort(engine string) int {
	switch strings.ToLower(engine) {
	case EngineMariaDB:
		return 3306
	case EnginePostgres:
		return 5432
	default:
		return 0
	}
}

// DBProvisionSpec is the per-instance contract the handler passes to engine
// configuration: which database to create, which user owns it (scoped to that
// db only), and an optional .sql file to run against the new database as the
// new user.
type DBProvisionSpec struct {
	TargetIP    string
	SSHUser     string
	DBName      string
	DBUser      string
	DBPassword  string
	LocalSQL    string // optional path to a .sql file on the manager host
	RemoteSQL   string // remote path where the SQL file will be uploaded
}

// ConfigureMariaDB creates the requested database, a user scoped to that
// database, and (if LocalSQL is provided) uploads + runs the .sql file as
// that user. Assumes the template image already has mariadb-server installed
// and root@'%' configured with DBRootPassword. Idempotent for re-runs of the
// same DBName/DBUser pair (drops and recreates).
//
// Writes the SQL to /tmp as the SSH user first (no sudo, /tmp is world-
// writable) and then runs mysql under a single sudo invocation. This avoids
// relying on sudo's credential cache, which is unreliable across multiple
// sudo calls inside a non-TTY SSH session.
func ConfigureMariaDB(spec DBProvisionSpec) error {
	if spec.DBName == "" || spec.DBUser == "" {
		return fmt.Errorf("DBName and DBUser are required")
	}
	sshCfg := SSHConfig{User: spec.SSHUser, Host: spec.TargetIP}

	script := fmt.Sprintf(`set -e

cat > /tmp/dbaas-init.sql <<'MARIADB_EOF'
CREATE DATABASE IF NOT EXISTS `+"`%s`"+`;
DROP USER IF EXISTS '%s'@'%%';
CREATE USER '%s'@'%%' IDENTIFIED BY '%s';
GRANT ALL PRIVILEGES ON `+"`%s`"+`.* TO '%s'@'%%';
FLUSH PRIVILEGES;
MARIADB_EOF

%s
`,
		spec.DBName,
		spec.DBUser, spec.DBUser, sqlSingleQuoteEscape(spec.DBPassword),
		spec.DBName, spec.DBUser,
		SudoBashWrap("mysql --protocol=socket -uroot < /tmp/dbaas-init.sql; rm -f /tmp/dbaas-init.sql"),
	)

	if output, err := RunSSHCommand(sshCfg, script); err != nil {
		return fmt.Errorf("configure mariadb on %s: %v - output: %s", spec.TargetIP, err, output)
	}

	if spec.LocalSQL == "" {
		return nil
	}
	return runUserSQLMariaDB(spec)
}

// EnsurePhpMyAdmin installs Apache + PHP + phpMyAdmin on a freshly-provisioned
// MariaDB VM if they aren't already present. Idempotent: apt-get install on
// already-installed packages is a no-op (~1 s), so when the multi-attach
// template has been rebuilt with phpMyAdmin baked in this becomes essentially
// free. When the template doesn't have it (default state of pre-existing
// templates), this brings the VM up to spec automatically.
//
// Requires the VM to have a NAT NIC (nic2 nat) attached and reachable; the
// NAT bring-up snippet inside the script handles PCI rescan, dhcp, and a
// static-IP fallback against VBox NAT defaults. Debconf is pre-seeded so the
// phpmyadmin install runs fully unattended.
//
// The script runs as root via SudoRunScript to dodge sudo's credential cache.
// Output is bounded — we don't stream apt's full chatter to the manager log,
// only the final state and the success marker.
func EnsurePhpMyAdmin(targetIP, sshUser, dbRootPassword string) error {
	sshCfg := SSHConfig{User: sshUser, Host: targetIP}

	body := fmt.Sprintf(`set -e
export DEBIAN_FRONTEND=noninteractive

# Short-circuit when phpmyadmin is already installed AND the apache conf is
# enabled — re-runs after a template rebuild become near-instant no-ops.
if dpkg -l phpmyadmin 2>/dev/null | grep -q '^ii' && [ -e /etc/apache2/conf-enabled/phpmyadmin.conf ]; then
    echo "[phpmyadmin] already installed and enabled, skipping"
    exit 0
fi

%s

apt-get update
apt-get install -y apache2 php php-mysql php-mbstring php-zip php-gd php-curl php-xml php-json libapache2-mod-php

debconf-set-selections <<DEBCONF
phpmyadmin phpmyadmin/dbconfig-install boolean false
phpmyadmin phpmyadmin/reconfigure-webserver multiselect apache2
phpmyadmin phpmyadmin/mysql/admin-user string root
phpmyadmin phpmyadmin/mysql/admin-pass password %s
phpmyadmin phpmyadmin/app-password-confirm password %s
phpmyadmin phpmyadmin/mysql/app-pass password %s
DEBCONF

apt-get install -y phpmyadmin

if [ -f /etc/phpmyadmin/apache.conf ] && [ ! -e /etc/apache2/conf-enabled/phpmyadmin.conf ]; then
    ln -s /etc/phpmyadmin/apache.conf /etc/apache2/conf-enabled/phpmyadmin.conf
fi
a2enmod rewrite || true
a2enconf phpmyadmin || true

systemctl enable apache2 || true
systemctl restart apache2 || service apache2 restart
echo "[phpmyadmin] ready"
`,
		natBringUpScript(),
		sqlSingleQuoteEscape(dbRootPassword),
		sqlSingleQuoteEscape(dbRootPassword),
		sqlSingleQuoteEscape(dbRootPassword),
	)

	if output, err := RunSSHCommand(sshCfg, SudoRunScript(body)); err != nil {
		return fmt.Errorf("install phpmyadmin on %s: %v - output: %s", targetIP, err, output)
	}
	return nil
}

// runUserSQLMariaDB uploads the user's .sql file to /tmp and runs it against
// the newly-created database, authenticating as the per-instance user (proves
// the credentials actually work end-to-end).
func runUserSQLMariaDB(spec DBProvisionSpec) error {
	remoteSQL := spec.RemoteSQL
	if remoteSQL == "" {
		remoteSQL = path.Join("/tmp", "dbaas-user-"+spec.DBName+".sql")
	}

	scpCfg := SSHConfig{User: spec.SSHUser, Host: spec.TargetIP, RemotePath: remoteSQL}
	if output, err := CopyFileBySCP(scpCfg, spec.LocalSQL); err != nil {
		return fmt.Errorf("scp user sql to %s: %v - %s", spec.TargetIP, err, output)
	}

	sshCfg := SSHConfig{User: spec.SSHUser, Host: spec.TargetIP}
	run := fmt.Sprintf(`set -e
mysql -u'%s' -p'%s' -h 127.0.0.1 '%s' < %s
rm -f %s
`,
		shellEscape(spec.DBUser), shellEscape(spec.DBPassword), shellEscape(spec.DBName), remoteSQL, remoteSQL,
	)
	if output, err := RunSSHCommand(sshCfg, run); err != nil {
		return fmt.Errorf("run user sql on %s: %v - output: %s", spec.TargetIP, err, output)
	}
	return nil
}

// ConfigurePostgres creates the requested database, a non-superuser role that
// owns it (so DBeaver gets full control over its own schema while staying
// least-privilege against other databases), and optionally runs the user's
// .sql file. Assumes the template image already has postgresql installed and
// the postgres role configured with DBRootPassword.
//
// Uses the same write-then-single-sudo pattern as ConfigureMariaDB to dodge
// sudo's unreliable cache in non-TTY SSH sessions.
func ConfigurePostgres(spec DBProvisionSpec) error {
	if spec.DBName == "" || spec.DBUser == "" {
		return fmt.Errorf("DBName and DBUser are required")
	}
	sshCfg := SSHConfig{User: spec.SSHUser, Host: spec.TargetIP}

	script := fmt.Sprintf(`set -e

cat > /tmp/dbaas-init.sql <<'PG_EOF'
DROP DATABASE IF EXISTS "%s";
DROP ROLE IF EXISTS "%s";
CREATE ROLE "%s" WITH LOGIN PASSWORD '%s';
CREATE DATABASE "%s" OWNER "%s";
GRANT ALL PRIVILEGES ON DATABASE "%s" TO "%s";
PG_EOF

%s
`,
		spec.DBName,
		spec.DBUser,
		spec.DBUser, sqlSingleQuoteEscape(spec.DBPassword),
		spec.DBName, spec.DBUser,
		spec.DBName, spec.DBUser,
		SudoBashWrap("sudo -u postgres psql -v ON_ERROR_STOP=1 -f /tmp/dbaas-init.sql; rm -f /tmp/dbaas-init.sql"),
	)

	if output, err := RunSSHCommand(sshCfg, script); err != nil {
		return fmt.Errorf("configure postgres on %s: %v - output: %s", spec.TargetIP, err, output)
	}

	if spec.LocalSQL == "" {
		return nil
	}
	return runUserSQLPostgres(spec)
}

// runUserSQLPostgres scp's the .sql file to the VM and runs it against the
// new database as the new role (using libpq's PGPASSWORD env var to avoid
// interactive prompts).
func runUserSQLPostgres(spec DBProvisionSpec) error {
	remoteSQL := spec.RemoteSQL
	if remoteSQL == "" {
		remoteSQL = path.Join("/tmp", "dbaas-user-"+spec.DBName+".sql")
	}

	scpCfg := SSHConfig{User: spec.SSHUser, Host: spec.TargetIP, RemotePath: remoteSQL}
	if output, err := CopyFileBySCP(scpCfg, spec.LocalSQL); err != nil {
		return fmt.Errorf("scp user sql to %s: %v - %s", spec.TargetIP, err, output)
	}

	sshCfg := SSHConfig{User: spec.SSHUser, Host: spec.TargetIP}
	run := fmt.Sprintf(`set -e
PGPASSWORD='%s' psql -h 127.0.0.1 -U '%s' -d '%s' -v ON_ERROR_STOP=1 -f %s
rm -f %s
`,
		shellEscape(spec.DBPassword), shellEscape(spec.DBUser), shellEscape(spec.DBName), remoteSQL, remoteSQL,
	)
	if output, err := RunSSHCommand(sshCfg, run); err != nil {
		return fmt.Errorf("run user sql on %s: %v - output: %s", spec.TargetIP, err, output)
	}
	return nil
}

// sqlSingleQuoteEscape doubles single quotes so the input is safe inside a
// single-quoted SQL literal in both MariaDB and PostgreSQL.
func sqlSingleQuoteEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// shellEscape escapes single quotes for inclusion inside a single-quoted bash
// argument: ' → '\''. Caller wraps in single quotes around the result.
func shellEscape(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}
