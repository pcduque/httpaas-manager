package models

type WebInstance struct {
	ID        int    `json:"id"`
	HostName  string `json:"host_name"`
	Domain    string `json:"domain"`
	FQDN      string `json:"fqdn"`
	IPAddress string `json:"ip_address"`
	VMName    string `json:"vm_name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}