package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"haproxy-manager/models"
	"haproxy-manager/storage"
	"haproxy-manager/utils"
)

func GetBalancers(w http.ResponseWriter, r *http.Request) {
	balancers, err := storage.LoadBalancers()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error loading balancers")
		return
	}

	utils.WriteJSON(w, http.StatusOK, balancers)
}

func CreateBalancer(w http.ResponseWriter, r *http.Request) {
	var input models.Balancer

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if input.Name == "" || input.IPAddress == "" || input.FrontendPort <= 0 || input.Algorithm == "" {
		utils.WriteError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	balancers, err := storage.LoadBalancers()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error loading balancers")
		return
	}

	nextID := 1
	for _, b := range balancers {
		if b.ID >= nextID {
			nextID = b.ID + 1
		}
	}

	input.ID = nextID
	balancers = append(balancers, input)

	if err := storage.SaveBalancers(balancers); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error saving balancer")
		return
	}

	utils.WriteJSON(w, http.StatusCreated, input)
}

func UpdateBalancer(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var input models.Balancer
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	balancers, err := storage.LoadBalancers()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error loading balancers")
		return
	}

	found := false
	for i, b := range balancers {
		if b.ID == id {
			input.ID = id
			balancers[i] = input
			found = true
			break
		}
	}

	if !found {
		utils.WriteError(w, http.StatusNotFound, "balancer not found")
		return
	}

	if err := storage.SaveBalancers(balancers); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error saving balancer")
		return
	}

	utils.WriteJSON(w, http.StatusOK, input)
}

func DeleteBalancer(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	balancers, err := storage.LoadBalancers()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error loading balancers")
		return
	}

	newBalancers := make([]models.Balancer, 0)
	found := false

	for _, b := range balancers {
		if b.ID == id {
			found = true
			continue
		}
		newBalancers = append(newBalancers, b)
	}

	if !found {
		utils.WriteError(w, http.StatusNotFound, "balancer not found")
		return
	}

	if err := storage.SaveBalancers(newBalancers); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "error deleting balancer")
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "balancer deleted",
	})
}