package models

type BackendServer struct {
	ID         int    `json:"id"`
	BalancerID int    `json:"balancer_id"`
	Name       string `json:"name"`
	IPAddress  string `json:"ip_address"`
	Port       int    `json:"port"`
}