package main

import (
	"fmt"
	"net/http"

	"httpaas-manager/config"
	"httpaas-manager/handlers"
	"httpaas-manager/services"
	"httpaas-manager/utils"
)

func instancesRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handlers.ListInstances(w, r)
	case http.MethodPost:
		handlers.CreateInstance(w, r)
	case http.MethodDelete:
		handlers.DeleteInstance(w, r)
	default:
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func main() {
	cfg := config.Load()
	services.InitVBox(cfg)
	handlers.Init(cfg)

	http.HandleFunc("/instances", instancesRoute)

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	addr := ":" + cfg.HTTPPort
	fmt.Printf("HTTPaaS Manager iniciado en http://0.0.0.0%s (cloud %s)\n", addr, cfg.Domain)
	fmt.Printf("VBoxManage mode: %s\n", services.VBoxMode())

	if err := http.ListenAndServe(addr, nil); err != nil {
		panic(err)
	}
}
