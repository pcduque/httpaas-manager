package models

// DBInstance is the persisted record for one DBaaS deployment. HostName is
// the server-level identifier (used for the VM name and DNS A record);
// DBName is the SQL-level database the user asked us to create on that
// server, decoupled so two users can host different databases under similar
// hostnames without colliding. DBUser is scoped to DBName (no GRANT ALL on
// *.* / SUPERUSER), matching the principle of least privilege the parcial-3
// spec calls for.
type DBInstance struct {
	HostName   string   `json:"hostname"`
	Engine     string   `json:"engine"`
	VMName     string   `json:"vm_name"`
	FQDN       string   `json:"fqdn"`
	IP         string   `json:"ip"`
	Port       int      `json:"port"`
	DBName     string   `json:"db_name"`
	DBUser     string   `json:"db_user"`
	DBPassword string   `json:"db_password"`
	CreatedAt  string   `json:"created_at"`
	Status     string   `json:"status"`
	Logs       []string `json:"logs"`
}
