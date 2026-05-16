# httpaas-manager — HTTPaaS + DBaaS sobre VirtualBox

Orquestador en Go que aprovisiona instancias virtualizadas para dos servicios
sobre el mismo host VirtualBox:

- **HTTPaaS**: sitios web Apache desplegados desde un `.zip` subido por el usuario.
- **DBaaS** (parcial 3): bases de datos **MariaDB** o **PostgreSQL** con usuario
  y schema definidos por el usuario, ejecutando opcionalmente un `.sql` inicial.

Cada instancia obtiene su propia VM, IP estática en la red Host-Only, y un
registro A en el BIND9 autoritativo del dominio `cloud.local`.

---

## Arquitectura

```
            ┌──────────────────────────────────────────┐
            │  Windows host (VirtualBox 7.2)           │
            │                                          │
            │  Host-Only adapter: 192.168.10.1/24      │
            │                                          │
            │  ┌──────────────────────────────────┐    │
            │  │ httpaas-manager (este binario)   │    │
            │  │ :8080 web UI + API REST          │    │
            │  └────────────┬─────────────────────┘    │
            │               │ VBoxManage local         │
            │               ▼                          │
            │  ┌──────────────────────────────────┐    │
            │  │  Multi-attach base VDIs          │    │
            │  │  ├ debian-web1.vdi  (HTTPaaS)    │    │
            │  │  ├ debian-mariadb.vdi (DBaaS)    │    │
            │  │  └ debian-postgres.vdi (DBaaS)   │    │
            │  └──────────────────────────────────┘    │
            │                                          │
            │  Cada instancia clonada arranca con      │
            │  un differencing image privado sobre     │
            │  la base correspondiente.                │
            └──────────────────────────────────────────┘
```

- **DNS autoritativo**: VM `debian-autoritativo` en `192.168.10.10` (BIND9,
  zona `cloud.local`, actualizada por `nsupdate` con la clave `rndc.key`).
- **Pool de IPs**: `192.168.10.30+` (compartido entre HTTPaaS y DBaaS).
- **IP temporal de boot**: `192.168.10.20` — cada clon arranca aquí antes de
  ser renumerado a su IP estática final.

---

## Prerrequisitos

1. **VirtualBox 7.2+** en el host Windows con `VBoxManage` en el path por
   defecto (`C:\Program Files\Oracle\VirtualBox\`).
2. Adaptador Host-Only `VirtualBox Host-Only Ethernet Adapter` con IP
   `192.168.10.1/24` y DHCP **deshabilitado**.
3. VM base **`debian-web1`** con Debian 13 CLI, usuario `orlay`, OpenSSH
   server, y `apt-get install apache2 unzip` ya hecho. El disco
   `debian-web1.vdi` debe estar en modo **multiattach**.
4. VM **`debian-autoritativo`** corriendo, alcanzable en `192.168.10.10`,
   con BIND9 sirviendo la zona `cloud.local` y `rndc.key` legible por root.
5. Llave SSH (`~/.ssh/id_ed25519` + `.pub`) — el manager la usa para
   autenticarse contra los guests sin password.
6. Go 1.26+ (solo para compilar).

---

## Configuración (variables de entorno)

Todas tienen default razonable; sobreescribe solo si tu setup difiere.

| Variable | Default | Para qué |
|---|---|---|
| `HTTPAAS_PORT` | `8080` | Puerto del web UI / API |
| `DOMAIN` | `cloud.local` | Zona DNS |
| `DNS_AUTH_IP` | `192.168.10.10` | Host del BIND9 autoritativo |
| `DNS_SSH_USER` | `orlay` | Usuario SSH del servidor DNS |
| `TEMPLATE_INITIAL_IP` | `192.168.10.20` | IP temporal de clones recién booteados |
| `IP_PREFIX` | `192.168.10.` | Subred del pool de instancias |
| `HOSTONLY_IF` | `VirtualBox Host-Only Ethernet Adapter` | Adaptador host-only |
| `GUEST_SSH_USER` | `orlay` | Usuario SSH dentro de las VMs |
| `GUEST_SSH_PASSWORD` | `1234567` | Password fallback si la llave no carga |
| `GUEST_SSH_KEY` | `~/.ssh/id_ed25519` | Llave privada para SSH a las VMs |
| `BASE_VDI_PATH` | `…\VirtualBox VMs\debian-web1.vdi` | Base HTTPaaS |
| `MARIADB_BASE_VDI` | `…\VirtualBox VMs\debian-mariadb.vdi` | Base DBaaS MariaDB |
| `POSTGRES_BASE_VDI` | `…\VirtualBox VMs\debian-postgres.vdi` | Base DBaaS PostgreSQL |
| `MARIADB_PORT` / `POSTGRES_PORT` | `3306` / `5432` | Puertos publicados |
| `DB_ROOT_PASSWORD` | `root123` | Password del superusuario DB (root / postgres) |
| `VBOX_LOCAL` | `true` (en Windows) | `false` para invocar `VBoxManage` vía SSH al host Windows |

---

## Preparación de plantillas (una sola vez por motor)

Las plantillas DBaaS NO existen al inicio — el manager las deriva de
`debian-web1.vdi` con un subcomando dedicado:

```bash
go build -o httpaas-manager.exe .

# Cada comando: ~5-10 min (clonemedium 2.6 GB + apt install + finalize)
./httpaas-manager.exe --prepare-mariadb-template
./httpaas-manager.exe --prepare-postgres-template
```

El flow interno:

1. `clonemedium debian-web1.vdi → debian-<engine>.vdi`.
2. Crea una VM temporal (`dbaas-prep-<engine>`) con NIC1 Host-Only + NIC2 NAT.
3. Bootea, hace **PCI rescan** dentro del guest (workaround para VBox 7.2 que
   a veces no enumera la NIC2 al boot), sube la NIC NAT con DHCP y, si falla,
   IP estática `10.0.2.15/24` con gateway `10.0.2.2`.
4. `apt-get install` del motor + configuración (`bind-address = 0.0.0.0` /
   `listen_addresses = '*'`, usuario superuser remoto, password de root, llave
   SSH del manager en `authorized_keys`).
5. Apaga la VM, detacha el disco, `modifymedium --type multiattach`, re-attacha
   a la prep VM (crea el primer differencing → la prep VM queda como **dueña**
   del multi-attach, requisito de VBox).
6. La prep VM queda **registrada y apagada** indefinidamente — no la borres,
   sin ella el multi-attach se rompe.

---

## Levantar el manager

```bash
./httpaas-manager.exe
# HTTPaaS + DBaaS Manager iniciado en http://0.0.0.0:8080 (cloud cloud.local)
# VBoxManage mode: local (C:\Program Files\Oracle\VirtualBox\VBoxManage.exe)
```

Web UI:

- `http://localhost:8080/` — dashboard **HTTPaaS** (subir `.zip`, listar
  instancias Apache, start/stop/eliminar).
- `http://localhost:8080/dbaas.html` — dashboard **DBaaS** (crear base con
  motor + nombre BD + usuario + password + `.sql` opcional, ver logs,
  eliminar).

---

## API REST

### HTTPaaS

| Método | Ruta | Cuerpo |
|---|---|---|
| `GET` | `/instances` | — |
| `POST` | `/instances` | multipart: `host_name`, `site_zip` |
| `DELETE` | `/instances?host_name=…` | — |
| `POST` | `/instances/start?host_name=…` | — |
| `POST` | `/instances/stop?host_name=…` | — |
| `POST` | `/instances/restart?host_name=…` | — |

### DBaaS

| Método | Ruta | Cuerpo |
|---|---|---|
| `GET` | `/db-instances` | — |
| `POST` | `/db-instances` | multipart: `host_name`, `engine`, `db_name`, `db_user`, `db_password`, `sql_file` (opcional) |
| `DELETE` | `/db-instances?host_name=…` | — |
| `POST` | `/db-instances/start?host_name=…` | — |
| `POST` | `/db-instances/stop?host_name=…` | — |
| `POST` | `/db-instances/restart?host_name=…` | — |
| `GET` | `/db-instances/logs?host_name=…` | — devuelve el log del pipeline |

Ejemplo de creación DBaaS:

```bash
curl -X POST http://127.0.0.1:8080/db-instances \
  -F "host_name=ventas" \
  -F "engine=mariadb" \
  -F "db_name=ventas_db" \
  -F "db_user=ventas_user" \
  -F "db_password=v3nt4s-pwd" \
  -F "sql_file=@schema.sql"
```

Restricciones de validación:

- `host_name`: minúsculas, dígitos y `-` únicamente.
- `db_name`, `db_user`: minúsculas, dígitos, `_-` únicamente.
- `db_password`: 4-64 chars del set `a-z A-Z 0-9 . _ - ! @ # $`.

---

## Pipeline de aprovisionamiento (DBaaS)

Para cada `POST /db-instances`:

1. Reserva la próxima IP libre del pool y guarda el placeholder con estado
   `creating`.
2. Goroutine asíncrona:
   1. `VBoxManage createvm` + `storageattach` al multi-attach base del motor
      → VBox crea differencing image privado.
   2. `startvm headless` y espera SSH en `TemplateInitialIP`.
   3. Cambia hostname al FQDN (`<host>.mdb.cloud.local` o `<host>.pg.cloud.local`),
      reasigna IP estática y reinicia la red.
   4. Espera SSH en la IP final.
   5. `nsupdate` al BIND9 autoritativo registrando el A record.
   6. Crea la base de datos solicitada, el usuario con privilegios scoped a
      esa BD, y ejecuta el `.sql` inicial si fue subido.
   7. Marca la instancia como `running`.

Si cualquier paso falla, el estado pasa a `error` y el log de la instancia
guarda el detalle (visible desde el botón "Logs" del dashboard).

---

## Conexión desde DBeaver / cliente externo

Cualquier instancia `running` es alcanzable desde la red Host-Only (cualquier
host con ruta a `192.168.10.0/24`):

- **MariaDB**: `jdbc:mysql://<ip>:3306/<db_name>`
- **PostgreSQL**: `jdbc:postgresql://<ip>:5432/<db_name>`

Las credenciales son las que se ingresaron al crear la base (visibles también
en el dashboard, botón "ver" del campo Contraseña).

---

## Detalles non-obvios aprendidos durante la implementación

Estas tres trampas no son evidentes en la documentación de VBox/Debian; las
dejo documentadas para que el siguiente debug sea más corto:

1. **`modifymedium --type multiattach` requiere VM-dueña con child UUID.**
   Si conviertes una VDI a multi-attach sin un VM que la tenga adjunta (con
   differencing image), futuros `storageattach` fallan con
   `VBOX_E_INVALID_OBJECT_STATE` y el mensaje engañoso "VirtualBox 4.0 or
   later". La receta en `services/template_service.go::finalizeTemplate`
   garantiza el child: detach → modifymedium → re-attach → dejar la prep VM
   registrada para siempre.

2. **VBox 7.2 + PIIX3 + Debian 13 no siempre enumera NIC2 al boot.**
   Aunque `VBoxManage showvminfo` confirma `nic2=nat`, el kernel del guest
   solo ve `enp0s3`. Fix: `echo 1 > /sys/bus/pci/rescan` antes de buscar
   `enp0s8` (ver `natBringUpScript` en `services/template_service.go`).

3. **El cache de credenciales de sudo no es confiable en sesiones SSH sin
   TTY.** El patrón "primear con `sudo -S -v` y después usar `sudo` sin
   password" falla intermitentemente con "a terminal is required to read the
   password". Fix: `SudoRunScript` (en `services/ssh_service.go`) escribe el
   script a `/tmp` como el usuario SSH y lo ejecuta en una **única**
   invocación `sudo -S bash <script>` — el cuerpo corre como root sin
   sudos anidados.

---

## Layout del proyecto

```
config/                  Carga de configuración (env vars)
models/                  WebInstance, DBInstance (formato JSON persistido)
storage/                 Persistencia en *.json (instancias HTTPaaS y DBaaS)
handlers/                HTTP handlers (CRUD + lifecycle por servicio)
services/
  ssh_service.go         SSH/SCP con key auth + helpers de sudo
  vbox_service.go        Wrapper de VBoxManage (local o remoto)
  apache_service.go      ConfigureClone (hostname/IP) y deploy de .zip
  dns_service.go         AddDNSRecord / RemoveDNSRecord vía nsupdate
  template_service.go    PrepareMariaDBTemplate / PreparePostgresTemplate
  db_service.go          ConfigureMariaDB / ConfigurePostgres (crea BD + user + corre SQL)
static/
  index.html             Dashboard HTTPaaS
  dbaas.html             Dashboard DBaaS
main.go                  Wiring de rutas y flags CLI
utils/response.go        Helpers JSON
```

Archivos persistentes en runtime (gitignored vía `.gitignore`):

- `instancias.json`, `db_instancias.json` — registros de instancias.
- `uploads/`, `db-uploads/` — archivos subidos por usuarios.
