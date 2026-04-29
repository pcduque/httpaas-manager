package main

import (
	"fmt"
	"net/http"

	"haproxy-manager/handlers"
	"haproxy-manager/utils"
)

func balancersRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handlers.GetBalancers(w, r)
	case http.MethodPost:
		handlers.CreateBalancer(w, r)
	case http.MethodPut:
		handlers.UpdateBalancer(w, r)
	case http.MethodDelete:
		handlers.DeleteBalancer(w, r)
	default:
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func serversRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handlers.GetServers(w, r)
	case http.MethodPost:
		handlers.CreateServer(w, r)
	case http.MethodPut:
		handlers.UpdateServer(w, r)
	case http.MethodDelete:
		handlers.DeleteServer(w, r)
	default:
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func applyHAProxyRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	handlers.ApplyFullHAProxyConfig(w, r)
}

func autoscaleConfigRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handlers.GetAutoScaleConfigHandler(w, r)
	case http.MethodPost:
		handlers.SaveAutoScaleConfigHandler(w, r)
	default:
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func autoscaleStateRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	handlers.GetAutoScaleStateHandler(w, r)
}

func autoscaleCheckRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	handlers.CheckAutoScaleHandler(w, r)
}

func stressStartRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	handlers.StartStressHandler(w, r)
}

func stressStopRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	handlers.StopStressHandler(w, r)
}

func autoscaleScaleOutRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	handlers.ManualScaleOutHandler(w, r)
}

func autoscaleScaleInRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	handlers.ManualScaleInHandler(w, r)
}

func autoscaleStartRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	handlers.StartAutoScaleHandler(w, r)
}

func autoscaleStopRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	handlers.StopAutoScaleHandler(w, r)
}

func main() {
	http.HandleFunc("/balancers", balancersRoute)
	http.HandleFunc("/servers", serversRoute)
	http.HandleFunc("/haproxy/apply", applyHAProxyRoute)

	http.HandleFunc("/autoscale/config", autoscaleConfigRoute)
	http.HandleFunc("/autoscale/state", autoscaleStateRoute)
	http.HandleFunc("/autoscale/check", autoscaleCheckRoute)

	http.HandleFunc("/stress/start", stressStartRoute)
	http.HandleFunc("/stress/stop", stressStopRoute)

	http.HandleFunc("/autoscale/scale-out", autoscaleScaleOutRoute)
	http.HandleFunc("/autoscale/scale-in", autoscaleScaleInRoute)

	http.HandleFunc("/autoscale/start", autoscaleStartRoute)
	http.HandleFunc("/autoscale/stop", autoscaleStopRoute)

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	fmt.Println("Servidor iniciado en http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}