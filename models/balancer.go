package models

type Balancer struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	IPAddress    string `json:"ip_address"`
	FrontendPort int    `json:"frontend_port"`
	Algorithm    string `json:"algorithm"`
	SSHUser      string `json:"ssh_user"`
	SSHPath      string `json:"ssh_path"`
}