package models

type AutoScaleConfig struct {
	Enabled            bool     `json:"enabled"`
	HighThreshold      float64  `json:"high_threshold"`
	LowThreshold       float64  `json:"low_threshold"`
	SampleSeconds      int      `json:"sample_seconds"`
	SustainSeconds     int      `json:"sustain_seconds"`
	MinInstances       int      `json:"min_instances"`
	MaxInstances       int      `json:"max_instances"`
	BalancerID         int      `json:"balancer_id"`
	TemplateVMName     string   `json:"template_vm_name"`
	BaseDiskNamePrefix string   `json:"base_disk_name_prefix"`
	AppPort            int      `json:"app_port"`
	SSHUser            string   `json:"ssh_user"`
	AvailableHosts     []string `json:"available_hosts"`
}