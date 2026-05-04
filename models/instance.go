package models

type WebInstance struct {
	HostName  string `json:"hostname"`
	IP        string `json:"ip"`
	VMName    string `json:"vm_name"`
	CreatedAt string `json:"created_at"`
	Status    string `json:"status"`
}
