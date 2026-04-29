package handlers

import (
	"encoding/json"
	"net/http"

	"haproxy-manager/services"
	"haproxy-manager/utils"
)

func StartStressHandler(w http.ResponseWriter, r *http.Request) {
	var req services.StressRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	msg, err := services.StartStress(req)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": msg,
	})
}

func StopStressHandler(w http.ResponseWriter, r *http.Request) {
	var req services.StressRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	msg, err := services.StopStress(req)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": msg,
	})
}