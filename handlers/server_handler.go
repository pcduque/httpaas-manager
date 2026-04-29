package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"haproxy-manager/models"
	"haproxy-manager/storage"
	"haproxy-manager/utils"
)

func GetServers(w http.ResponseWriter, r *http.Request) {
	balancerIDStr := r.URL.Query().Get("balancer_id")
	balancerID, err := strconv.Atoi(balancerIDStr)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid balancer_id")
		return
	}

	servers, err := storage.LoadServers()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error loading servers")
		return
	}

	filtered := make([]models.BackendServer, 0)
	for _, s := range servers {
		if s.BalancerID == balancerID {
			filtered = append(filtered, s)
		}
	}

	utils.WriteJSON(w, http.StatusOK, filtered)
}

func CreateServer(w http.ResponseWriter, r *http.Request) {
	var input models.BackendServer

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if input.BalancerID <= 0 || input.Name == "" || input.IPAddress == "" || input.Port <= 0 {
		utils.WriteError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	servers, err := storage.LoadServers()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error loading servers")
		return
	}

	nextID := 1
	for _, s := range servers {
		if s.ID >= nextID {
			nextID = s.ID + 1
		}
	}

	input.ID = nextID
	servers = append(servers, input)

	if err := storage.SaveServers(servers); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error saving server")
		return
	}

	utils.WriteJSON(w, http.StatusCreated, input)
}

func UpdateServer(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var input models.BackendServer
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	servers, err := storage.LoadServers()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error loading servers")
		return
	}

	found := false
	for i, s := range servers {
		if s.ID == id {
			input.ID = id
			servers[i] = input
			found = true
			break
		}
	}

	if !found {
		utils.WriteError(w, http.StatusNotFound, "server not found")
		return
	}

	if err := storage.SaveServers(servers); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error saving server")
		return
	}

	utils.WriteJSON(w, http.StatusOK, input)
}

func DeleteServer(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	servers, err := storage.LoadServers()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error loading servers")
		return
	}

	newServers := make([]models.BackendServer, 0)
	found := false

	for _, s := range servers {
		if s.ID == id {
			found = true
			continue
		}
		newServers = append(newServers, s)
	}

	if !found {
		utils.WriteError(w, http.StatusNotFound, "server not found")
		return
	}

	if err := storage.SaveServers(newServers); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error deleting server")
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "server deleted",
	})
}