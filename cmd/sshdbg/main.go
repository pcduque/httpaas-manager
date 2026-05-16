package main

import (
	"fmt"
	"os"
	"strings"

	"httpaas-manager/services"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: sshdbg <remote-command>")
		os.Exit(2)
	}
	cmd := strings.Join(os.Args[1:], " ")
	user := os.Getenv("SSH_USER")
	if user == "" {
		user = "orlay"
	}
	host := os.Getenv("SSH_HOST")
	if host == "" {
		host = "192.168.10.20"
	}
	services.InitGuestSSHKey(os.Getenv("SSH_KEY"))
	out, err := services.RunSSHCommand(services.SSHConfig{User: user, Host: host}, cmd)
	fmt.Print(out)
	if !strings.HasSuffix(out, "\n") {
		fmt.Println()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ssh error] %v\n", err)
		os.Exit(1)
	}
}
