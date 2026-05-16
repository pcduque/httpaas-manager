package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"httpaas-manager/models"
	"httpaas-manager/services"
	"httpaas-manager/storage"
	"httpaas-manager/utils"
)

// dbProvisionMu serializes the boot-time configuration of new DB VMs because
// every fresh instance comes up on Cfg.TemplateInitialIP before being
// reassigned its final static IP. Concurrent provisioning would collide on
// that shared IP. Distinct from provisionMu so HTTPaaS and DBaaS provisioning
// can run in parallel.
var dbProvisionMu sync.Mutex

// dbAllocateMu protects the read-then-save sequence used to reserve the next
// static IP across BOTH HTTPaaS and DBaaS instances; the two pools share the
// 192.168.10.x/24 host-only subnet.
var dbAllocateMu sync.Mutex

func ListDBInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := storage.LoadDBInstances()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	utils.WriteJSON(w, http.StatusOK, instances)
}

// CreateDBInstance accepts a multipart/form-data request with fields:
//
//	host_name, db_name, db_user, db_password, engine, sql_file (optional .sql)
//
// Persists a "creating" placeholder, saves the .sql file (if any) under
// db-uploads/<host>-<engine>.sql, and runs the slow VM provisioning in a
// goroutine. Returns 202 with the placeholder so the UI can show a transient
// status badge and poll until the pipeline finishes.
func CreateDBInstance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}

	hostName := sanitizeDBHostName(r.FormValue("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required (only a-z, 0-9, '-' allowed)")
		return
	}

	engine := strings.ToLower(strings.TrimSpace(r.FormValue("engine")))
	if engine != services.EngineMariaDB && engine != services.EnginePostgres {
		utils.WriteError(w, http.StatusBadRequest, "engine must be 'mariadb' or 'postgres'")
		return
	}

	dbName := sanitizeDBIdentifier(r.FormValue("db_name"))
	if dbName == "" {
		utils.WriteError(w, http.StatusBadRequest, "db_name is required (a-z, 0-9, '_-' only)")
		return
	}

	dbUser := sanitizeDBIdentifier(r.FormValue("db_user"))
	if dbUser == "" {
		utils.WriteError(w, http.StatusBadRequest, "db_user is required (a-z, 0-9, '_-' only)")
		return
	}

	dbPass := strings.TrimSpace(r.FormValue("db_password"))
	if !isAllowedDBPassword(dbPass) {
		utils.WriteError(w, http.StatusBadRequest, "db_password must be 4-64 chars from a-z A-Z 0-9 . _ - ! @ # $")
		return
	}

	localSQLPath, err := saveOptionalSQLUpload(r, hostName, engine)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	baseVDI, err := dbBaseVDIForEngine(engine)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	vmName := dbVMName(hostName, engine)
	subdomain := services.EngineSubdomain(engine)
	fqdn := fmt.Sprintf("%s.%s.%s", hostName, subdomain, strings.TrimSuffix(Cfg.Domain, "."))
	port := services.EngineDefaultPort(engine)

	dbAllocateMu.Lock()
	dbInstances, err := storage.LoadDBInstances()
	if err != nil {
		dbAllocateMu.Unlock()
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, ins := range dbInstances {
		if ins.HostName == hostName && ins.Engine == engine {
			dbAllocateMu.Unlock()
			utils.WriteError(w, http.StatusConflict, "an instance with this name and engine already exists")
			return
		}
	}
	staticIP, err := allocateNextIPCrossPool()
	if err != nil {
		dbAllocateMu.Unlock()
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	placeholder := models.DBInstance{
		HostName:   hostName,
		Engine:     engine,
		VMName:     vmName,
		FQDN:       fqdn,
		IP:         staticIP,
		Port:       port,
		DBName:     dbName,
		DBUser:     dbUser,
		DBPassword: dbPass,
		CreatedAt:  time.Now().Format(time.RFC3339),
		Status:     "creating",
		Logs:       []string{},
	}
	saved, err := storage.AddDBInstance(placeholder)
	dbAllocateMu.Unlock()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go func(ins models.DBInstance, baseVDI, sqlPath string) {
		if err := provisionDBInstance(ins, baseVDI, sqlPath); err != nil {
			fmt.Printf("provision db %s/%s failed: %v\n", ins.Engine, ins.HostName, err)
			_ = storage.AppendDBInstanceLog(ins.HostName, "ERROR: "+err.Error())
			storage.UpdateDBInstanceStatus(ins.HostName, "error")
			return
		}
		_ = storage.AppendDBInstanceLog(ins.HostName, "provisioning complete")
		storage.UpdateDBInstanceStatus(ins.HostName, "running")
		if sqlPath != "" {
			_ = os.Remove(sqlPath)
		}
	}(saved, baseVDI, localSQLPath)

	utils.WriteJSON(w, http.StatusAccepted, saved)
}

// saveOptionalSQLUpload stages the .sql multipart file (if present) under
// db-uploads/<host>-<engine>.sql so the provisioning goroutine can SCP it
// after the VM is reachable. Returns "" with nil error if the user did not
// upload a file. Rejects non-.sql filenames and oversized uploads.
func saveOptionalSQLUpload(r *http.Request, hostName, engine string) (string, error) {
	file, header, err := r.FormFile("sql_file")
	if err == http.ErrMissingFile {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("sql_file upload: %w", err)
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".sql") {
		return "", fmt.Errorf("sql_file must be a .sql file (got %q)", header.Filename)
	}

	if err := os.MkdirAll("db-uploads", 0755); err != nil {
		return "", fmt.Errorf("create db-uploads dir: %w", err)
	}
	dstPath := filepath.Join("db-uploads", fmt.Sprintf("%s-%s.sql", hostName, engine))
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", dstPath, err)
	}
	defer dst.Close()
	if _, err := dst.ReadFrom(file); err != nil {
		return "", fmt.Errorf("write %s: %w", dstPath, err)
	}
	return dstPath, nil
}

// provisionDBInstance runs the full create pipeline for a DB instance: clone
// VM from multi-attach base, configure hostname/IP, register DNS, create the
// requested database + scoped user, and optionally run the user's .sql file.
//
// dbProvisionMu is held only across the steps that depend on the shared
// Cfg.TemplateInitialIP (boot + hostname/IP reconfigure). A local flag tracks
// whether we already unlocked it so the deferred safety unlock doesn't run
// after the explicit one — unlocking twice would panic.
func provisionDBInstance(ins models.DBInstance, baseVDI, localSQLPath string) error {
	logStep := func(line string) {
		_ = storage.AppendDBInstanceLog(ins.HostName, line)
	}

	dbProvisionMu.Lock()
	provisionLocked := true
	defer func() {
		if provisionLocked {
			dbProvisionMu.Unlock()
		}
	}()

	logStep("creating VM from base " + baseVDI)
	if err := services.CreateVMFromBase(services.VMConfig{
		VMName:      ins.VMName,
		BaseVDIPath: baseVDI,
		HostOnlyIF:  Cfg.HostOnlyIF,
		MemoryMB:    Cfg.VMMemoryMB,
		CPUs:        Cfg.VMCPUs,
		OSType:      Cfg.VMOSType,
	}); err != nil {
		return err
	}

	logStep("waiting SSH on template IP " + Cfg.TemplateInitialIP)
	if err := waitForSSH(Cfg.TemplateInitialIP, Cfg.SSHUser); err != nil {
		return err
	}

	subdomainHost := ins.HostName + "." + services.EngineSubdomain(ins.Engine)

	logStep("configuring hostname and static IP " + ins.IP)
	if err := services.ConfigureClone(
		Cfg.TemplateInitialIP, Cfg.SSHUser, subdomainHost, ins.IP, Cfg.IPPrefix, Cfg.Domain,
	); err != nil {
		return err
	}

	logStep("waiting SSH on static IP " + ins.IP)
	if err := waitForSSH(ins.IP, Cfg.SSHUser); err != nil {
		return err
	}
	dbProvisionMu.Unlock()
	provisionLocked = false

	logStep("registering DNS record " + ins.FQDN + " -> " + ins.IP)
	if err := services.AddDNSRecord(services.DNSConfig{
		Host: Cfg.DNSAuthoritativeIP,
		User: Cfg.DNSSSHUser,
		Zone: Cfg.Domain,
	}, subdomainHost, ins.IP); err != nil {
		return err
	}

	logStep(fmt.Sprintf("provisioning database %q with user %q", ins.DBName, ins.DBUser))
	spec := services.DBProvisionSpec{
		TargetIP:   ins.IP,
		SSHUser:    Cfg.SSHUser,
		DBName:     ins.DBName,
		DBUser:     ins.DBUser,
		DBPassword: ins.DBPassword,
		LocalSQL:   localSQLPath,
	}
	switch ins.Engine {
	case services.EngineMariaDB:
		if err := services.ConfigureMariaDB(spec); err != nil {
			return err
		}
	case services.EnginePostgres:
		if err := services.ConfigurePostgres(spec); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown engine: %s", ins.Engine)
	}

	if localSQLPath != "" {
		logStep("user SQL applied from " + filepath.Base(localSQLPath))
	}

	return nil
}

// DeleteDBInstance marks the record as "deleting", returns 202, and runs the
// VM/DNS/store teardown in a goroutine. The instance disappears from the
// table only after the cleanup completes.
func DeleteDBInstance(w http.ResponseWriter, r *http.Request) {
	hostName := sanitizeDBHostName(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}

	target, found, err := findDBInstance(hostName)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		utils.WriteError(w, http.StatusNotFound, "instance not found")
		return
	}

	if _, _, err := storage.UpdateDBInstanceStatus(hostName, "deleting"); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go func(ins models.DBInstance) {
		subdomainHost := ins.HostName + "." + services.EngineSubdomain(ins.Engine)
		_ = storage.AppendDBInstanceLog(ins.HostName, "removing VM "+ins.VMName)
		if err := services.UnregisterVM(ins.VMName); err != nil {
			fmt.Printf("unregister %s failed: %v\n", ins.VMName, err)
		}
		_ = storage.AppendDBInstanceLog(ins.HostName, "removing DNS record")
		if err := services.RemoveDNSRecord(services.DNSConfig{
			Host: Cfg.DNSAuthoritativeIP,
			User: Cfg.DNSSSHUser,
			Zone: Cfg.Domain,
		}, subdomainHost); err != nil {
			fmt.Printf("remove dns %s failed: %v\n", ins.FQDN, err)
		}
		if _, _, err := storage.RemoveDBInstanceByHostName(ins.HostName); err != nil {
			fmt.Printf("remove store %s failed: %v\n", ins.HostName, err)
		}
	}(target)

	utils.WriteJSON(w, http.StatusAccepted, map[string]string{
		"deleting": hostName,
	})
}

// StartDBInstance powers on the VM, marks "starting", and only flips to
// "running" once SSH is reachable on the static IP. Re-adds the DNS record
// because StopDBInstance removes it.
func StartDBInstance(w http.ResponseWriter, r *http.Request) {
	hostName := sanitizeDBHostName(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}

	target, found, err := findDBInstance(hostName)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		utils.WriteError(w, http.StatusNotFound, "instance not found")
		return
	}

	if err := services.StartVM(target.VMName); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, _, err := storage.UpdateDBInstanceStatus(hostName, "starting")
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go func(ins models.DBInstance) {
		subdomainHost := ins.HostName + "." + services.EngineSubdomain(ins.Engine)
		_ = storage.AppendDBInstanceLog(ins.HostName, "waiting SSH after start")
		if err := waitForSSH(ins.IP, Cfg.SSHUser); err != nil {
			_ = storage.AppendDBInstanceLog(ins.HostName, "SSH did not respond: "+err.Error())
			storage.UpdateDBInstanceStatus(ins.HostName, "stopped")
			return
		}
		if err := services.AddDNSRecord(services.DNSConfig{
			Host: Cfg.DNSAuthoritativeIP,
			User: Cfg.DNSSSHUser,
			Zone: Cfg.Domain,
		}, subdomainHost, ins.IP); err != nil {
			fmt.Printf("add dns %s on start: %v\n", ins.FQDN, err)
		}
		_ = storage.AppendDBInstanceLog(ins.HostName, "instance running")
		storage.UpdateDBInstanceStatus(ins.HostName, "running")
	}(target)

	utils.WriteJSON(w, http.StatusAccepted, updated)
}

// StopDBInstance sends ACPI shutdown, waits up to ~5 s for the guest to power
// off, escalates to forced poweroff, and removes the DNS record so clients
// don't keep resolving to a now-unreachable IP.
func StopDBInstance(w http.ResponseWriter, r *http.Request) {
	hostName := sanitizeDBHostName(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}

	target, found, err := findDBInstance(hostName)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		utils.WriteError(w, http.StatusNotFound, "instance not found")
		return
	}

	if err := services.StopVM(target.VMName); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, _, err := storage.UpdateDBInstanceStatus(hostName, "stopping")
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go func(ins models.DBInstance) {
		subdomainHost := ins.HostName + "." + services.EngineSubdomain(ins.Engine)
		powerDownAndConfirm(ins.VMName)
		if err := services.RemoveDNSRecord(services.DNSConfig{
			Host: Cfg.DNSAuthoritativeIP,
			User: Cfg.DNSSSHUser,
			Zone: Cfg.Domain,
		}, subdomainHost); err != nil {
			fmt.Printf("remove dns %s on stop: %v\n", ins.FQDN, err)
		}
		_ = storage.AppendDBInstanceLog(ins.HostName, "instance stopped")
		storage.UpdateDBInstanceStatus(ins.HostName, "stopped")
	}(target)

	utils.WriteJSON(w, http.StatusAccepted, updated)
}

// RestartDBInstance cleanly reboots the VM via ACPI + start. IP, FQDN and DNS
// record stay valid; the status briefly moves through "restarting" while we
// confirm SSH is back.
func RestartDBInstance(w http.ResponseWriter, r *http.Request) {
	hostName := sanitizeDBHostName(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}

	target, found, err := findDBInstance(hostName)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		utils.WriteError(w, http.StatusNotFound, "instance not found")
		return
	}

	updated, _, err := storage.UpdateDBInstanceStatus(hostName, "restarting")
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go func(ins models.DBInstance) {
		_ = storage.AppendDBInstanceLog(ins.HostName, "restarting VM")
		if err := services.RestartVM(ins.VMName); err != nil {
			_ = storage.AppendDBInstanceLog(ins.HostName, "restart failed: "+err.Error())
			storage.UpdateDBInstanceStatus(ins.HostName, "error")
			return
		}
		if err := waitForSSH(ins.IP, Cfg.SSHUser); err != nil {
			_ = storage.AppendDBInstanceLog(ins.HostName, "SSH did not respond after restart: "+err.Error())
			storage.UpdateDBInstanceStatus(ins.HostName, "stopped")
			return
		}
		_ = storage.AppendDBInstanceLog(ins.HostName, "instance running after restart")
		storage.UpdateDBInstanceStatus(ins.HostName, "running")
	}(target)

	utils.WriteJSON(w, http.StatusAccepted, updated)
}

// GetDBInstanceLogs returns the stored pipeline log for a single instance,
// most recent line last. The dashboard renders this in a modal.
func GetDBInstanceLogs(w http.ResponseWriter, r *http.Request) {
	hostName := sanitizeDBHostName(r.URL.Query().Get("host_name"))
	if hostName == "" {
		utils.WriteError(w, http.StatusBadRequest, "host_name is required")
		return
	}

	target, found, err := findDBInstance(hostName)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		utils.WriteError(w, http.StatusNotFound, "instance not found")
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"host_name": target.HostName,
		"engine":    target.Engine,
		"status":    target.Status,
		"logs":      target.Logs,
	})
}

func findDBInstance(hostName string) (models.DBInstance, bool, error) {
	instances, err := storage.LoadDBInstances()
	if err != nil {
		return models.DBInstance{}, false, err
	}
	for _, ins := range instances {
		if ins.HostName == hostName {
			return ins, true, nil
		}
	}
	return models.DBInstance{}, false, nil
}

func dbBaseVDIForEngine(engine string) (string, error) {
	switch engine {
	case services.EngineMariaDB:
		if Cfg.MariaDBBaseVDIPath == "" {
			return "", fmt.Errorf("MARIADB_BASE_VDI is not configured")
		}
		return Cfg.MariaDBBaseVDIPath, nil
	case services.EnginePostgres:
		if Cfg.PostgresBaseVDIPath == "" {
			return "", fmt.Errorf("POSTGRES_BASE_VDI is not configured")
		}
		return Cfg.PostgresBaseVDIPath, nil
	default:
		return "", fmt.Errorf("unknown engine: %s", engine)
	}
}

func dbVMName(hostName, engine string) string {
	sub := services.EngineSubdomain(engine)
	if sub == "" {
		return hostName
	}
	return hostName + "-" + sub
}

// allocateNextIPCrossPool reserves the next free static IP looking at BOTH
// HTTPaaS and DBaaS instance records, because they share the same /24. Called
// while holding dbAllocateMu.
func allocateNextIPCrossPool() (string, error) {
	used := make(map[string]bool)

	webInstances, err := storage.LoadInstances()
	if err != nil {
		return "", err
	}
	for _, ins := range webInstances {
		used[ins.IP] = true
	}

	dbInstances, err := storage.LoadDBInstances()
	if err != nil {
		return "", err
	}
	for _, ins := range dbInstances {
		used[ins.IP] = true
	}

	for octet := Cfg.IPStart; octet < 255; octet++ {
		candidate := fmt.Sprintf("%s%d", Cfg.IPPrefix, octet)
		if !used[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free IP in %s0/24", Cfg.IPPrefix)
}

func sanitizeDBHostName(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	input = strings.ReplaceAll(input, " ", "-")
	input = strings.ReplaceAll(input, "_", "-")
	clean := make([]rune, 0, len(input))
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			clean = append(clean, r)
		}
	}
	return string(clean)
}

func sanitizeDBIdentifier(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	clean := make([]rune, 0, len(input))
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			clean = append(clean, r)
		}
	}
	return string(clean)
}

// isAllowedDBPassword enforces a charset that is safe across both shell and
// SQL contexts (no backslashes, no quote characters beyond single-quote which
// is handled by sqlSingleQuoteEscape). 4-64 chars, conservative ASCII subset.
func isAllowedDBPassword(s string) bool {
	if len(s) < 4 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == '!' || r == '@' || r == '#' || r == '$':
		default:
			return false
		}
	}
	return true
}
