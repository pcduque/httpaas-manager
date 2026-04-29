package main

import (
	"fmt"
	"net/http"

	"httpaas-manager/handlers"
	"httpaas-manager/utils"
)

func instancesRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handlers.ListInstances(w, r)
	case http.MethodPost:
		handlers.CreateInstance(w, r)
	default:
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func main() {
	http.HandleFunc("/instances", instancesRoute)

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	fmt.Println("HTTPaaS Manager iniciado en http://localhost:8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}