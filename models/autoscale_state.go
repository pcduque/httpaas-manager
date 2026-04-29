package models

type AutoScaleState struct {
	LastCPUAverage   float64 `json:"last_cpu_average"`
	ActiveInstances  int     `json:"active_instances"`
	LastAction       string  `json:"last_action"`
	LastActionTime   string  `json:"last_action_time"`
	HighCounter      int     `json:"high_counter"`
	LowCounter       int     `json:"low_counter"`
	IsRunning        bool    `json:"is_running"`
}