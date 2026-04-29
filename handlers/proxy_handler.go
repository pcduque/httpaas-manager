package handlers

import (
	"net/http"

	"haproxy-manager/services"
	"haproxy-manager/utils"
)

func ApplyFullHAProxyConfig(w http.ResponseWriter, r *http.Request) {
	content, filePath, err := services.BuildAndSaveFullHAProxyConfig()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sshCfg := services.SSHConfig{
		User:       "server1",
		Host:       "192.168.1.6",
		RemotePath: "/home/server1",
	}

	if err := services.ApplyRemoteHAProxyConfig(sshCfg, filePath); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Full HAProxy configuration applied successfully",
		"config":  content,
		"path":    filePath,
	})
}