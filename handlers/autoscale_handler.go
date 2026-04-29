package handlers

import (
	"encoding/json"
	"net/http"

	"haproxy-manager/models"
	"haproxy-manager/services"
	"haproxy-manager/utils"
)

func GetAutoScaleConfigHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := services.GetAutoScaleConfig()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, cfg)
}

func SaveAutoScaleConfigHandler(w http.ResponseWriter, r *http.Request) {
	var cfg models.AutoScaleConfig

	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := services.SaveValidatedAutoScaleConfig(cfg); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "autoscale config saved successfully",
	})
}

func GetAutoScaleStateHandler(w http.ResponseWriter, r *http.Request) {
	state, err := services.GetAutoScaleState()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, state)
}

func CheckAutoScaleHandler(w http.ResponseWriter, r *http.Request) {
	state, details, err := services.CheckAutoScaleNow()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"state":   state,
		"details": details,
	})
}

func StartAutoScaleHandler(w http.ResponseWriter, r *http.Request) {
	err := services.StartAutoScaleLoop()
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "autoscaling loop started successfully",
	})
}

func StopAutoScaleHandler(w http.ResponseWriter, r *http.Request) {
	err := services.StopAutoScaleLoop()
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "autoscaling loop stopped successfully",
	})
}