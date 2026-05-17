package services

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// TemplatePrepConfig is the input contract for one-time template preparation.
// All fields come from the manager's config; this struct exists so the
// long-running prep flow doesn't depend on the global config package.
type TemplatePrepConfig struct {
	SourceVDI       string // existing debian-web1.vdi (must exist)
	TargetVDI       string // new .vdi to produce (must NOT exist)
	PrepVMName      string // VM name used during install AND kept as the multi-attach owner
	HostOnlyIF      string
	MemoryMB        int
	CPUs            int
	OSType          string
	InitialIP       string // IP the cloned guest comes up on (TemplateInitialIP)
	SSHUser         string
	DBRootPassword  string // baked-in DB root password (MariaDB root@'%', PostgreSQL postgres role)
	AuthorizedKey   string // OpenSSH public key line appended to authorized_keys
	SSHWaitAttempts int    // retries while waiting for SSH after boot
}

func (c TemplatePrepConfig) sshWait() int {
	if c.SSHWaitAttempts > 0 {
		return c.SSHWaitAttempts
	}
	return 30
}

// PrepareMariaDBTemplate produces TargetVDI from SourceVDI with mariadb-server
// installed, the SQL `root` user reachable from any host (root@'%') with the
// password baked into cfg.DBRootPassword, and the manager's SSH public key
// added to /home/<SSHUser>/.ssh/authorized_keys for passwordless SSH. The disk
// ends up in multi-attach mode owned by the prep VM. Idempotent at the
// boundaries: if TargetVDI already exists the call aborts.
func PrepareMariaDBTemplate(cfg TemplatePrepConfig) error {
	if err := prepareCloneAndBoot(cfg); err != nil {
		return err
	}

	installBody := fmt.Sprintf(`set -e
export DEBIAN_FRONTEND=noninteractive

%s

%s

apt-get update
apt-get install -y mariadb-server

CONF=/etc/mysql/mariadb.conf.d/50-server.cnf
sed -i 's/^[[:space:]]*bind-address.*/bind-address = 0.0.0.0/' "$CONF" || true
if ! grep -qE '^[[:space:]]*bind-address' "$CONF"; then
    echo 'bind-address = 0.0.0.0' >> "$CONF"
fi

systemctl restart mariadb || service mariadb restart

# Enable root with all privileges reachable from any host. The local
# unix-socket root stays peer-auth so the manager can keep using
# --protocol=socket for follow-on provisioning without a password.
mysql --protocol=socket -uroot <<'SQL'
DROP USER IF EXISTS 'root'@'%%';
CREATE USER 'root'@'%%' IDENTIFIED BY '%s';
GRANT ALL PRIVILEGES ON *.* TO 'root'@'%%' WITH GRANT OPTION;
FLUSH PRIVILEGES;
SQL

systemctl enable mariadb || true
echo "[install] mariadb install complete"

# phpMyAdmin stack: Apache + PHP + the package itself. debconf answers below
# are pre-seeded so the install is fully unattended (no Apache picker, no
# dbconfig-common DB bootstrap — phpMyAdmin will talk to the local mariadb
# as root via the password baked above).
echo "[install] installing apache2 + php + phpmyadmin"
apt-get install -y apache2 php php-mysql php-mbstring php-zip php-gd php-curl php-xml php-json libapache2-mod-php

debconf-set-selections <<'DEBCONF'
phpmyadmin phpmyadmin/dbconfig-install boolean false
phpmyadmin phpmyadmin/reconfigure-webserver multiselect apache2
phpmyadmin phpmyadmin/mysql/admin-user string root
phpmyadmin phpmyadmin/mysql/admin-pass password %s
phpmyadmin phpmyadmin/app-password-confirm password %s
phpmyadmin phpmyadmin/mysql/app-pass password %s
DEBCONF

apt-get install -y phpmyadmin

# Some Debian builds ship the apache snippet but do not enable it; guarantee
# both the conf and the rewrite module are active so /phpmyadmin resolves.
if [ -f /etc/phpmyadmin/apache.conf ] && [ ! -e /etc/apache2/conf-enabled/phpmyadmin.conf ]; then
    ln -s /etc/phpmyadmin/apache.conf /etc/apache2/conf-enabled/phpmyadmin.conf
fi
a2enmod rewrite || true
a2enconf phpmyadmin || true

# Allow phpMyAdmin to be reached over plain HTTP from anywhere on the
# host-only subnet (no public exposure: VBox host-only network is isolated).
systemctl enable apache2 || true
systemctl restart apache2 || service apache2 restart

echo "[install] phpmyadmin ready at http://<host>/phpmyadmin"
`,
		natBringUpScript(),
		sshAuthorizedKeyScript(cfg),
		sqlSingleQuoteEscape(cfg.DBRootPassword),
		sqlSingleQuoteEscape(cfg.DBRootPassword),
		sqlSingleQuoteEscape(cfg.DBRootPassword),
		sqlSingleQuoteEscape(cfg.DBRootPassword),
	)

	if err := runOnPrepVM(cfg, SudoRunScript(installBody), "MariaDB install"); err != nil {
		return err
	}

	return finalizeTemplate(cfg)
}

// PreparePostgresTemplate produces TargetVDI from SourceVDI with postgresql
// installed, listening on all interfaces, network md5 auth permitted, the
// built-in `postgres` superuser given a password (so DBeaver and friends can
// reach it from outside the unix socket), and the manager's SSH public key
// added to authorized_keys for passwordless SSH. Disk ends up in multi-attach
// mode owned by the prep VM.
func PreparePostgresTemplate(cfg TemplatePrepConfig) error {
	if err := prepareCloneAndBoot(cfg); err != nil {
		return err
	}

	installBody := fmt.Sprintf(`set -e
export DEBIAN_FRONTEND=noninteractive

%s

%s

apt-get update
apt-get install -y postgresql postgresql-contrib

PG_VERSION=$(ls /etc/postgresql 2>/dev/null | sort -V | tail -n1)
if [ -z "$PG_VERSION" ]; then
    echo "ERROR: postgresql not installed after apt" >&2
    exit 1
fi
PG_CONF=/etc/postgresql/$PG_VERSION/main/postgresql.conf
PG_HBA=/etc/postgresql/$PG_VERSION/main/pg_hba.conf

sed -i "s/^[[:space:]]*#\?listen_addresses.*/listen_addresses = '*'/" "$PG_CONF"
if ! grep -qE "^[[:space:]]*listen_addresses" "$PG_CONF"; then
    echo "listen_addresses = '*'" >> "$PG_CONF"
fi

if ! grep -qE '^host[[:space:]]+all[[:space:]]+all[[:space:]]+0\.0\.0\.0/0[[:space:]]+md5' "$PG_HBA"; then
    echo "host all all 0.0.0.0/0 md5" >> "$PG_HBA"
fi

systemctl restart postgresql || service postgresql restart
for i in $(seq 1 15); do
    if su -s /bin/sh postgres -c 'psql -tAc "SELECT 1"' >/dev/null 2>&1; then
        break
    fi
    sleep 2
done

# Give the postgres superuser a password so DBeaver / external clients can
# authenticate via TCP. Local socket access (which the manager uses for
# follow-on provisioning) keeps working under peer auth.
su -s /bin/sh postgres -c "psql -v ON_ERROR_STOP=1" <<'SQL'
ALTER ROLE postgres WITH PASSWORD '%s';
SQL

systemctl enable postgresql || true
echo "[install] postgresql install complete"
`,
		natBringUpScript(),
		sshAuthorizedKeyScript(cfg),
		sqlSingleQuoteEscape(cfg.DBRootPassword),
	)

	if err := runOnPrepVM(cfg, SudoRunScript(installBody), "PostgreSQL install"); err != nil {
		return err
	}

	return finalizeTemplate(cfg)
}

// prepareCloneAndBoot performs the steps shared by both engines: clone the
// source VDI to the target path, create a temporary VM around it, boot, and
// wait until SSH responds. Leaves the VM running so the caller can apt-install
// over SSH.
func prepareCloneAndBoot(cfg TemplatePrepConfig) error {
	if cfg.SourceVDI == "" || cfg.TargetVDI == "" || cfg.PrepVMName == "" {
		return fmt.Errorf("SourceVDI, TargetVDI and PrepVMName are required")
	}
	if _, err := os.Stat(cfg.SourceVDI); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot stat source VDI %s locally (may live on remote host): %v\n", cfg.SourceVDI, err)
	}
	if _, err := os.Stat(cfg.TargetVDI); err == nil {
		return fmt.Errorf("target VDI %s already exists; remove it manually if you want to rebuild the template", cfg.TargetVDI)
	}

	fmt.Printf("→ Cloning %s to %s ...\n", cfg.SourceVDI, cfg.TargetVDI)
	if err := CloneMediumDisk(cfg.SourceVDI, cfg.TargetVDI); err != nil {
		return err
	}

	fmt.Printf("→ Creating prep VM %s ...\n", cfg.PrepVMName)
	if err := CreateVMWithDirectDisk(VMConfig{
		VMName:      cfg.PrepVMName,
		BaseVDIPath: cfg.TargetVDI,
		HostOnlyIF:  cfg.HostOnlyIF,
		MemoryMB:    cfg.MemoryMB,
		CPUs:        cfg.CPUs,
		OSType:      cfg.OSType,
	}); err != nil {
		return err
	}

	fmt.Printf("→ Waiting for SSH on %s ...\n", cfg.InitialIP)
	if err := waitForTemplateSSH(cfg.InitialIP, cfg.SSHUser, cfg.sshWait()); err != nil {
		return fmt.Errorf("template VM did not respond on SSH: %w", err)
	}
	return nil
}

// runOnPrepVM runs the engine-specific install script over SSH and surfaces
// the combined output if anything fails.
func runOnPrepVM(cfg TemplatePrepConfig, script, label string) error {
	fmt.Printf("→ Running %s on %s ...\n", label, cfg.InitialIP)
	sshCfg := SSHConfig{User: cfg.SSHUser, Host: cfg.InitialIP}
	if output, err := RunSSHCommand(sshCfg, script); err != nil {
		return fmt.Errorf("%s failed: %v\n--- remote output ---\n%s", label, err, output)
	}
	return nil
}

// finalizeTemplate converts the prep VDI to multi-attach mode and keeps the
// prep VM around as the owner. Steps:
//
//  1. Power off the prep VM.
//  2. Detach the disk (medium stays in the registry).
//  3. modifymedium --type multiattach — works while medium is registered but
//     not attached to any VM.
//  4. Re-attach the (now multi-attach) base to the prep VM. VirtualBox
//     auto-creates a differencing image for the prep VM, which makes the prep
//     VM the "owner" of the multi-attach base. Without an owner, future
//     storageattach calls on fresh VMs fail with VBOX_E_INVALID_OBJECT_STATE
//     ("the media type 'MultiAttach' can only be attached to machines that
//     were created with VirtualBox 4.0 or later" — misleading message).
//
// The prep VM stays REGISTERED (powered off, name like `dbaas-prep-mariadb`)
// indefinitely. It serves no purpose beyond pinning the multi-attach base.
// Users shouldn't delete it without also un-multi-attaching the base.
func finalizeTemplate(cfg TemplatePrepConfig) error {
	fmt.Println("→ Shutting down prep VM (ACPI, then poweroff fallback) ...")
	if out, err := RunVBoxManage("controlvm", cfg.PrepVMName, "acpipowerbutton"); err != nil {
		if !strings.Contains(out, "is not currently running") {
			fmt.Printf("warning: ACPI shutdown %s: %v - %s\n", cfg.PrepVMName, err, out)
		}
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if state, _ := VMState(cfg.PrepVMName); state == "poweroff" {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if state, _ := VMState(cfg.PrepVMName); state != "poweroff" {
		if err := ForcePowerOffVM(cfg.PrepVMName); err != nil {
			fmt.Printf("warning: forced poweroff: %v\n", err)
		}
		time.Sleep(3 * time.Second)
	}

	fmt.Println("→ Detaching disk from prep VM (medium stays registered) ...")
	if out, err := RunVBoxManage("storageattach", cfg.PrepVMName,
		"--storagectl", "SATA",
		"--port", "0",
		"--device", "0",
		"--type", "hdd",
		"--medium", "none"); err != nil {
		return fmt.Errorf("detach disk from %s: %v - %s", cfg.PrepVMName, err, out)
	}

	fmt.Println("→ Converting target VDI to multi-attach ...")
	if out, err := RunVBoxManage("modifymedium", "disk", cfg.TargetVDI, "--type", "multiattach"); err != nil {
		return fmt.Errorf("modifymedium multiattach %s: %v - %s", cfg.TargetVDI, err, out)
	}

	fmt.Println("→ Re-attaching multi-attach base to prep VM (creates the owner differencing image) ...")
	if out, err := RunVBoxManage("storageattach", cfg.PrepVMName,
		"--storagectl", "SATA",
		"--port", "0",
		"--device", "0",
		"--type", "hdd",
		"--medium", cfg.TargetVDI); err != nil {
		return fmt.Errorf("re-attach multi-attach base to %s: %v - %s", cfg.PrepVMName, err, out)
	}

	fmt.Printf("✓ Template ready: %s (multi-attach, owner=%s)\n", cfg.TargetVDI, cfg.PrepVMName)
	return nil
}

// sshAuthorizedKeyScript returns a shell snippet that drops the manager's
// public key into ~/.ssh/authorized_keys for the prep SSH user. Designed to
// run AS ROOT (no sudo prefixes). If cfg.AuthorizedKey is empty the snippet
// is a no-op so the prep flow still works without a configured key
// (password auth fallback). Idempotent: re-adds the key only if absent.
func sshAuthorizedKeyScript(cfg TemplatePrepConfig) string {
	if strings.TrimSpace(cfg.AuthorizedKey) == "" {
		return "echo '[ssh-key] no AuthorizedKey provided; skipping authorized_keys setup'"
	}
	user := cfg.SSHUser
	if user == "" {
		user = "orlay"
	}
	return fmt.Sprintf(`PUBKEY='%s'
HOME_DIR=$(getent passwd %s | cut -d: -f6)
install -d -m 700 -o %s -g %s "$HOME_DIR/.ssh"
if ! grep -qxF "$PUBKEY" "$HOME_DIR/.ssh/authorized_keys" 2>/dev/null; then
    echo "$PUBKEY" >> "$HOME_DIR/.ssh/authorized_keys"
fi
chmod 600 "$HOME_DIR/.ssh/authorized_keys"
chown %s:%s "$HOME_DIR/.ssh/authorized_keys"
echo "[ssh-key] authorized_keys updated for %s"`,
		strings.TrimSpace(cfg.AuthorizedKey), user, user, user, user, user, user)
}

// natBringUpScript returns the shell snippet that brings up the NAT-attached
// second NIC so the prep VM has outbound internet for apt. Designed to run
// AS ROOT (no sudo prefixes) — the caller is expected to wrap the whole
// install in a single sudo invocation via SudoRunScript.
//
// Strategy:
//   1. Try dhcpcd (present on debian-web1, but might be inactive).
//   2. Try /sbin/ifup with a dhcp stanza in /etc/network/interfaces.d/.
//   3. Fall back to static IP from VirtualBox NAT's well-known defaults
//      (10.0.2.0/24, gateway 10.0.2.2, DNS 10.0.2.3).
//
// The directly connected 192.168.10.0/24 route on enp0s3 keeps SSH alive
// even after a default route is added via NAT — return traffic prefers the
// more specific connected route.
func natBringUpScript() string {
	return `# Some VBox 7.2 + PIIX3 + Debian 13 combinations fail to enumerate the NIC2
# device at boot — the kernel sees only the first Ethernet controller. A
# manual PCI rescan re-runs enumeration and exposes the late-added device,
# which then shows up as enp0s8 (or similar) in /sys/class/net/.
echo "[nat-bringup] PCI rescan (workaround for missed NIC2 enumeration)"
echo 1 > /sys/bus/pci/rescan 2>/dev/null || true
sleep 2

NAT_IF=$(ls /sys/class/net/ 2>/dev/null | grep -vE '^(lo|enp0s3)$' | head -n1)
if [ -z "$NAT_IF" ]; then
    echo "ERROR: cannot find NAT NIC inside guest after PCI rescan" >&2
    echo "[nat-bringup] dmesg PCI tail:" >&2
    dmesg 2>/dev/null | grep -E '^\[' | grep -iE 'pci|ether' | tail -10 >&2 || true
    exit 1
fi
echo "[nat-bringup] NIC=$NAT_IF"

/sbin/ip link set "$NAT_IF" up 2>&1 || true

if [ -x /sbin/dhcpcd ]; then
    echo "[nat-bringup] trying dhcpcd"
    /sbin/dhcpcd -t 15 "$NAT_IF" 2>&1 | tail -10 || true
fi

if ! /sbin/ip -4 addr show "$NAT_IF" 2>/dev/null | grep -q 'inet '; then
    echo "[nat-bringup] dhcpcd path didn't yield IPv4; trying ifup"
    mkdir -p /etc/network/interfaces.d
    cat > "/etc/network/interfaces.d/${NAT_IF}" <<NETEOF
allow-hotplug ${NAT_IF}
iface ${NAT_IF} inet dhcp
NETEOF
    if ! grep -q '^source /etc/network/interfaces.d/' /etc/network/interfaces; then
        echo 'source /etc/network/interfaces.d/*' >> /etc/network/interfaces
    fi
    if [ -x /sbin/ifup ]; then
        /sbin/ifup "$NAT_IF" 2>&1 | tail -10 || true
    fi
fi

if ! /sbin/ip -4 addr show "$NAT_IF" 2>/dev/null | grep -q 'inet '; then
    echo "[nat-bringup] no DHCP success; assigning static 10.0.2.15/24 (VBox NAT default)"
    /sbin/ip addr add 10.0.2.15/24 dev "$NAT_IF" 2>&1 || true
fi
if ! /sbin/ip -4 route show default | grep -q .; then
    /sbin/ip route add default via 10.0.2.2 dev "$NAT_IF" 2>&1 || true
fi

if ! grep -q '^nameserver 10\.0\.2\.3' /etc/resolv.conf 2>/dev/null; then
    echo 'nameserver 10.0.2.3' >> /etc/resolv.conf
fi
if ! grep -q '^nameserver 8\.8\.8\.8' /etc/resolv.conf 2>/dev/null; then
    echo 'nameserver 8.8.8.8' >> /etc/resolv.conf
fi

echo "[nat-bringup] interface state:"
/sbin/ip -4 addr show "$NAT_IF" 2>&1 || true
echo "[nat-bringup] routes:"
/sbin/ip -4 route 2>&1 || true
echo "[nat-bringup] resolv.conf:"
cat /etc/resolv.conf 2>&1 | head -10

if ! /sbin/ip -4 addr show "$NAT_IF" 2>/dev/null | grep -q 'inet '; then
    echo "ERROR: $NAT_IF still has no IPv4 address after all attempts" >&2
    exit 1
fi

if ! curl -sf --max-time 20 http://deb.debian.org/ -o /dev/null; then
    echo "ERROR: $NAT_IF has IPv4 but http://deb.debian.org is unreachable" >&2
    /sbin/ip -4 route 2>&1 >&2 || true
    cat /etc/resolv.conf 2>&1 >&2 || true
    exit 1
fi
echo "[nat-bringup] internet OK"`
}

// waitForTemplateSSH polls SSH on the freshly booted VM. Distinct from the
// handler's waitForSSH so the template flow can pick its own attempt count
// (apt updates over slow links can take a while before the VM is reachable).
func waitForTemplateSSH(ip, user string, attempts int) error {
	cfg := SSHConfig{User: user, Host: ip}
	for i := 0; i < attempts; i++ {
		_, err := RunSSHCommand(cfg, "echo ok")
		if err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("ssh not available on %s after %d attempts", ip, attempts)
}
