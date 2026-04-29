package handlers

import (
	"net/http"

	"haproxy-manager/services"
	"haproxy-manager/utils"
)

func ManualScaleOutHandler(w http.ResponseWriter, r *http.Request) {
	result, err := services.ManualScaleOut()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, result)
}

func ManualScaleInHandler(w http.ResponseWriter, r *http.Request) {
	result, err := services.ManualScaleIn()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, result)
}