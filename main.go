package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

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

func dbInstancesRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handlers.ListDBInstances(w, r)
	case http.MethodPost:
		handlers.CreateDBInstance(w, r)
	case http.MethodDelete:
		handlers.DeleteDBInstance(w, r)
	default:
		utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func main() {
	multiattach := flag.Bool("multiattach", false, "convert the base VDI to multi-attach mode and exit")
	prepareMariaDB := flag.Bool("prepare-mariadb-template", false, "clone the Apache base VDI, install MariaDB+phpMyAdmin, and leave the new VDI multi-attach ready")
	preparePostgres := flag.Bool("prepare-postgres-template", false, "clone the Apache base VDI, install PostgreSQL, and leave the new VDI multi-attach ready")
	flag.Parse()

	cfg := config.Load()
	services.InitVBox(cfg)
	services.InitGuestSSHKey(cfg.GuestSSHKeyPath)
	handlers.Init(cfg)

	if *multiattach {
		changed, err := services.EnsureMultiAttachBase(cfg.BaseVDIPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "multi-attach conversion failed: %v\n", err)
			os.Exit(1)
		}
		if changed {
			fmt.Printf("Base VDI %s set to multi-attach.\n", cfg.BaseVDIPath)
		} else {
			fmt.Printf("Base VDI %s already in multi-attach mode.\n", cfg.BaseVDIPath)
		}
		return
	}

	if *prepareMariaDB || *preparePostgres {
		authorizedKey, err := services.ReadAuthorizedKey(cfg.GuestSSHKeyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read public key for guest SSH (%s.pub): %v\n", cfg.GuestSSHKeyPath, err)
			fmt.Fprintln(os.Stderr, "  template prep will continue without baking a public key (password SSH will still work)")
			authorizedKey = ""
		}

		if *prepareMariaDB {
			if err := services.PrepareMariaDBTemplate(services.TemplatePrepConfig{
				SourceVDI:      cfg.BaseVDIPath,
				TargetVDI:      cfg.MariaDBBaseVDIPath,
				PrepVMName:     "dbaas-prep-mariadb",
				HostOnlyIF:     cfg.HostOnlyIF,
				MemoryMB:       cfg.VMMemoryMB,
				CPUs:           cfg.VMCPUs,
				OSType:         cfg.VMOSType,
				InitialIP:      cfg.TemplateInitialIP,
				SSHUser:        cfg.SSHUser,
				DBRootPassword: cfg.DBRootPassword,
				AuthorizedKey:  authorizedKey,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "prepare MariaDB template failed: %v\n", err)
				os.Exit(1)
			}
			return
		}

		if *preparePostgres {
			if err := services.PreparePostgresTemplate(services.TemplatePrepConfig{
				SourceVDI:      cfg.BaseVDIPath,
				TargetVDI:      cfg.PostgresBaseVDIPath,
				PrepVMName:     "dbaas-prep-postgres",
				HostOnlyIF:     cfg.HostOnlyIF,
				MemoryMB:       cfg.VMMemoryMB,
				CPUs:           cfg.VMCPUs,
				OSType:         cfg.VMOSType,
				InitialIP:      cfg.TemplateInitialIP,
				SSHUser:        cfg.SSHUser,
				DBRootPassword: cfg.DBRootPassword,
				AuthorizedKey:  authorizedKey,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "prepare PostgreSQL template failed: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if changed, err := services.EnsureMultiAttachBase(cfg.BaseVDIPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not verify multi-attach base: %v\n", err)
	} else if changed {
		fmt.Printf("Base VDI %s set to multi-attach.\n", cfg.BaseVDIPath)
	}

	// HTTPaaS routes (existing)
	http.HandleFunc("/instances", instancesRoute)
	http.HandleFunc("/instances/start", postOnly(handlers.StartInstance))
	http.HandleFunc("/instances/stop", postOnly(handlers.StopInstance))
	http.HandleFunc("/instances/restart", postOnly(handlers.RestartInstance))

	// DBaaS routes
	http.HandleFunc("/db-instances", dbInstancesRoute)
	http.HandleFunc("/db-instances/start", postOnly(handlers.StartDBInstance))
	http.HandleFunc("/db-instances/stop", postOnly(handlers.StopDBInstance))
	http.HandleFunc("/db-instances/restart", postOnly(handlers.RestartDBInstance))
	http.HandleFunc("/db-instances/logs", getOnly(handlers.GetDBInstanceLogs))
	http.HandleFunc("/db-instances/dbeaver", postOnly(handlers.RegisterInDBeaver))

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	addr := ":" + cfg.HTTPPort
	fmt.Printf("HTTPaaS + DBaaS Manager iniciado en http://0.0.0.0%s (cloud %s)\n", addr, cfg.Domain)
	fmt.Printf("VBoxManage mode: %s\n", services.VBoxMode())

	if err := http.ListenAndServe(addr, nil); err != nil {
		panic(err)
	}
}

func postOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h(w, r)
	}
}

func getOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			utils.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h(w, r)
	}
}
