package services

import (
	"fmt"
	"sync"
	"time"

	"haproxy-manager/models"
	"haproxy-manager/storage"
)

var (
	autoScaleRunning bool
	autoScaleStopCh  chan struct{}
	autoScaleMutex   sync.Mutex
)

func ValidateAutoScaleConfig(cfg models.AutoScaleConfig) error {
	if cfg.BalancerID <= 0 {
		return fmt.Errorf("balancer_id is required")
	}
	if cfg.HighThreshold <= 0 || cfg.HighThreshold > 100 {
		return fmt.Errorf("high_threshold must be between 0 and 100")
	}
	if cfg.LowThreshold < 0 || cfg.LowThreshold >= 100 {
		return fmt.Errorf("low_threshold must be between 0 and 100")
	}
	if cfg.LowThreshold >= cfg.HighThreshold {
		return fmt.Errorf("low_threshold must be lower than high_threshold")
	}
	if cfg.SampleSeconds <= 0 {
		return fmt.Errorf("sample_seconds must be greater than 0")
	}
	if cfg.SustainSeconds <= 0 {
		return fmt.Errorf("sustain_seconds must be greater than 0")
	}
	if cfg.MinInstances <= 0 {
		return fmt.Errorf("min_instances must be greater than 0")
	}
	if cfg.MaxInstances < cfg.MinInstances {
		return fmt.Errorf("max_instances must be greater than or equal to min_instances")
	}
	return nil
}

func SaveValidatedAutoScaleConfig(cfg models.AutoScaleConfig) error {
	if err := ValidateAutoScaleConfig(cfg); err != nil {
		return err
	}

	return storage.SaveAutoScaleConfig(cfg)
}

func GetAutoScaleConfig() (models.AutoScaleConfig, error) {
	return storage.LoadAutoScaleConfig()
}

func GetAutoScaleState() (models.AutoScaleState, error) {
	return storage.LoadAutoScaleState()
}

func IsAutoScaleRunning() bool {
	autoScaleMutex.Lock()
	defer autoScaleMutex.Unlock()
	return autoScaleRunning
}

func StartAutoScaleLoop() error {
	autoScaleMutex.Lock()
	defer autoScaleMutex.Unlock()

	if autoScaleRunning {
		return fmt.Errorf("autoscaling is already running")
	}

	cfg, err := storage.LoadAutoScaleConfig()
	if err != nil {
		return err
	}

	if err := ValidateAutoScaleConfig(cfg); err != nil {
		return err
	}

	autoScaleStopCh = make(chan struct{})
	autoScaleRunning = true

	state, _ := storage.LoadAutoScaleState()
	state.IsRunning = true
	state.LastAction = "autoscaling_loop_started"
	state.LastActionTime = time.Now().Format(time.RFC3339)
	_ = storage.SaveAutoScaleState(state)

	go func() {
		ticker := time.NewTicker(time.Duration(cfg.SampleSeconds) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, _, err := CheckAutoScaleNow()
				if err != nil {
					state, _ := storage.LoadAutoScaleState()
					state.LastAction = "autoscaling_loop_error: " + err.Error()
					state.LastActionTime = time.Now().Format(time.RFC3339)
					_ = storage.SaveAutoScaleState(state)
				}

			case <-autoScaleStopCh:
				return
			}
		}
	}()

	return nil
}

func StopAutoScaleLoop() error {
	autoScaleMutex.Lock()
	defer autoScaleMutex.Unlock()

	if !autoScaleRunning {
		return fmt.Errorf("autoscaling is not running")
	}

	close(autoScaleStopCh)
	autoScaleRunning = false

	state, _ := storage.LoadAutoScaleState()
	state.IsRunning = false
	state.LastAction = "autoscaling_loop_stopped"
	state.LastActionTime = time.Now().Format(time.RFC3339)
	_ = storage.SaveAutoScaleState(state)

	return nil
}

func CheckAutoScaleNow() (models.AutoScaleState, []map[string]interface{}, error) {
	cfg, err := storage.LoadAutoScaleConfig()
	if err != nil {
		return models.AutoScaleState{}, nil, err
	}

	if err := ValidateAutoScaleConfig(cfg); err != nil {
		return models.AutoScaleState{}, nil, err
	}

	avgCPU, details, err := GetConfiguredAverageCPU(cfg)
	if err != nil {
		return models.AutoScaleState{}, nil, err
	}

	servers, err := GetServersByBalancerID(cfg.BalancerID)
	if err != nil {
		return models.AutoScaleState{}, nil, err
	}

	state, err := storage.LoadAutoScaleState()
	if err != nil {
		return models.AutoScaleState{}, nil, err
	}

	now := time.Now().Format(time.RFC3339)

	state.LastCPUAverage = avgCPU
	state.ActiveInstances = len(servers)
	state.IsRunning = IsAutoScaleRunning()

	if !cfg.Enabled {
		state.LastAction = "autoscaling_disabled"
		state.LastActionTime = now
		state.HighCounter = 0
		state.LowCounter = 0

		if err := storage.SaveAutoScaleState(state); err != nil {
			return models.AutoScaleState{}, nil, err
		}

		return state, details, nil
	}

	if avgCPU > cfg.HighThreshold {
		state.HighCounter += cfg.SampleSeconds
		state.LowCounter = 0

		if state.HighCounter >= cfg.SustainSeconds {
			_, actionErr := ManualScaleOut()
			if actionErr != nil {
				state.LastAction = "scale_out_failed: " + actionErr.Error()
				state.LastActionTime = now
			} else {
				state.LastAction = "scale_out_executed"
				state.LastActionTime = now

				updatedServers, _ := GetServersByBalancerID(cfg.BalancerID)
				state.ActiveInstances = len(updatedServers)
			}

			state.HighCounter = 0
			state.LowCounter = 0
		} else {
			state.LastAction = "high_cpu_detected_waiting"
			state.LastActionTime = now
		}

	} else if avgCPU < cfg.LowThreshold {
		state.LowCounter += cfg.SampleSeconds
		state.HighCounter = 0

		if state.LowCounter >= cfg.SustainSeconds {
			_, actionErr := ManualScaleIn()
			if actionErr != nil {
				state.LastAction = "scale_in_failed: " + actionErr.Error()
				state.LastActionTime = now
			} else {
				state.LastAction = "scale_in_executed"
				state.LastActionTime = now

				updatedServers, _ := GetServersByBalancerID(cfg.BalancerID)
				state.ActiveInstances = len(updatedServers)
			}

			state.HighCounter = 0
			state.LowCounter = 0
		} else {
			state.LastAction = "low_cpu_detected_waiting"
			state.LastActionTime = now
		}

	} else {
		state.HighCounter = 0
		state.LowCounter = 0
		state.LastAction = "within_thresholds"
		state.LastActionTime = now
	}

	if err := storage.SaveAutoScaleState(state); err != nil {
		return models.AutoScaleState{}, nil, err
	}

	return state, details, nil
}