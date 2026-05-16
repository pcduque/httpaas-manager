# Proyecto de Computación en la Nube — Servicio Administrado de Bases de Datos (DBaaS)

> Universidad del Quindío — Facultad de Ingeniería — Programa de Ingeniería de Sistemas y Cómputo
>
> Asignatura: Computación en la Nube
> Profesor: Carlos Eduardo Gómez Montoya
> Estudiantes: Pablo Cesar Duque · Brahian Andrés Arbeláez Aguirre · Orlay Andrés Molina Gómez
> Armenia, Quindío — 2026

---

## 1. Resumen ejecutivo

Este documento describe el proceso de implementación del **Servicio Administrado de Bases de Datos (DBaaS)** desarrollado como extensión del orquestador *HTTPaaS Manager* (parcial anterior), siguiendo los requerimientos del enunciado **Nota Parcial #3 — Implementación simplificada de un servicio administrado de base de datos en entorno local con Oracle VirtualBox y múltiples motores de base de datos en Linux**.

La plataforma permite al usuario solicitar, desde una interfaz web, una nueva base de datos sobre **MariaDB** o **PostgreSQL**: especifica el nombre de la BD, el usuario propietario, la contraseña y opcionalmente sube un archivo `.sql` con el esquema y los datos iniciales. El orquestador:

1. Clona una máquina virtual a partir de la plantilla del motor elegido (disco virtual en modo **multiconexión**).
2. Configura hostname e IP estática única.
3. Registra un A-record en el servidor **BIND9 autoritativo** (zona `cloud.local`).
4. Crea la base de datos, el usuario con privilegios scoped a esa base, y ejecuta el `.sql` inicial.
5. Devuelve al usuario los datos de conexión (host, puerto, usuario, contraseña) listos para usarse desde **DBeaver** o cualquier cliente externo.

El código está escrito en **Go** y empaquetado en un único binario que también orquesta la creación/eliminación/listado de las instancias y la generación una sola vez de las plantillas base.

---

## 2. Arquitectura de la solución

La solución se compone de cuatro tipos de nodos coexistiendo en la red Host-Only `192.168.10.0/24`:

| Nodo | Función | IP |
|---|---|---|
| Host Windows | Ejecuta el binario Go `httpaas-manager.exe` que invoca `VBoxManage` localmente | `192.168.10.1` (Host-Only adapter) |
| `debian-autoritativo` | Servidor BIND9 autoritativo para `cloud.local`, recibe `nsupdate` con TSIG | `192.168.10.10` |
| `dbaas-prep-mariadb` | VM "plantilla dueña" del disco multiconexión `debian-mariadb.vdi` (apagada) | n/a |
| `dbaas-prep-postgres` | VM "plantilla dueña" del disco multiconexión `debian-postgres.vdi` (apagada) | n/a |
| `<instancia>-mdb`, `<instancia>-pg` | VMs provisionadas a demanda, una por base solicitada | `192.168.10.30+` |

Cada instancia clonada arranca con un **disco diferencial** automático sobre el disco multiconexión correspondiente, lo que permite que la base de datos compartida ocupe ~3 GB en disco y cada instancia solo crezca según los cambios que aplique.

<<IMAGEN: Diagrama de arquitectura DBaaS (host Windows + DNS + 2 plantillas + N instancias) — puede dibujarse en draw.io o capturarse del README.md>>

---

## 3. Objetivo del proyecto

Diseñar e implementar un prototipo funcional de un servicio administrado de bases de datos que demuestre el aprovisionamiento, configuración y eliminación automatizados de bases de datos relacionales (MariaDB y PostgreSQL) sobre máquinas virtuales VirtualBox, simulando el funcionamiento básico de servicios cloud como Amazon RDS o Google Cloud SQL.

**Objetivos específicos:**

- Diseñar la arquitectura e identificar los componentes (red Host-Only, plantillas, DNS, orquestador, dashboard) y su interacción.
- Implementar y configurar los componentes de infraestructura: VMs Debian CLI, BIND9, plantillas con MariaDB y PostgreSQL preinstaladas, SSH con llaves públicas/privadas y disco virtual en modo multiconexión.
- Desarrollar los mecanismos de aprovisionamiento y gestión: dashboard web con creación, listado, ejecución de SQL inicial y eliminación de instancias.
- Diseñar y ejecutar un plan de pruebas end-to-end para verificar funcionamiento, disponibilidad y confiabilidad.

---

## 4. Componentes principales

### 4.1 Backend en Go (host Windows)

Servicio HTTP que corre en el host Windows porque debe invocar `VBoxManage.exe` para crear/iniciar/detener/eliminar las VMs. Está compilado como un único binario y comparte la base de código con el HTTPaaS Manager del parcial anterior (mismo `dns_service`, `ssh_service`, `vbox_service`, sumando los módulos DBaaS).

**Layout de paquetes (extensión DBaaS):**

| Paquete | Contenido nuevo / modificado |
|---|---|
| `config` | Vars de entorno: `MARIADB_BASE_VDI`, `POSTGRES_BASE_VDI`, `DB_ROOT_PASSWORD`, `GUEST_SSH_KEY` |
| `models` | `DBInstance` con campos `db_name`, `db_user`, `db_password`, `engine`, `fqdn`, `port`, `logs` |
| `storage` | `db_instance_store.go` — persistencia JSON análoga a `instance_store.go` |
| `services` | `db_service.go` (Configure MariaDB/Postgres + scoped grants + run user SQL), `template_service.go` (Prepare MariaDB/Postgres templates), helpers de SSH key auth y `SudoRunScript` |
| `handlers` | `db_instance_handler.go` — endpoints multipart `/db-instances` |
| `static` | `dbaas.html` — dashboard con tabla, modal de logs, modal "Conectar" con comandos copy-paste |

### 4.2 Plantillas Debian preconfiguradas (multiconexión)

Cada motor tiene su propia VDI base creada por el subcomando `--prepare-<motor>-template` del binario. Ambas se derivan de `debian-web1.vdi` (Debian 13 CLI + Apache + unzip + usuario `orlay` con sudo + OpenSSH).

**`debian-mariadb.vdi` (≈ 3 GB, multiattach)**
- `mariadb-server 11.8.6` instalado y habilitado.
- `bind-address = 0.0.0.0` en `/etc/mysql/mariadb.conf.d/50-server.cnf`.
- Usuario `root@'%'` con contraseña `root123` y `GRANT ALL PRIVILEGES ON *.* WITH GRANT OPTION`.
- Llave pública SSH del manager en `/home/orlay/.ssh/authorized_keys`.

**`debian-postgres.vdi` (≈ 3 GB, multiattach)**
- `postgresql 17` instalado y habilitado.
- `listen_addresses = '*'` en `postgresql.conf`, `host all all 0.0.0.0/0 md5` en `pg_hba.conf`.
- Rol `postgres` con contraseña `root123` para acceso TCP (peer auth local intacta).
- Llave pública SSH del manager en authorized_keys.

Las dos VDIs están en modo **MultiAttach** y son "propiedad" de la VM dueña (`dbaas-prep-mariadb` / `dbaas-prep-postgres`), que queda registrada y apagada indefinidamente — sin esa dueña, VirtualBox rechaza nuevos `storageattach` de la base multi-attach.

### 4.3 Servidor DNS `debian-autoritativo` (ns1)

Reutilizado del parcial anterior: BIND9 autoritativo para `cloud.local`, acepta `nsupdate` firmado con TSIG (`/etc/bind/rndc.key`). El manager invoca `nsupdate` por SSH dentro del propio ns1 para evitar exponer la clave por la red.

Convención de FQDN:
- MariaDB → `<host>.mdb.cloud.local`
- PostgreSQL → `<host>.pg.cloud.local`

### 4.4 Dashboard web (`/dbaas.html`)

- Formulario: motor, nombre del servidor, nombre de la BD, usuario, contraseña, `.sql` opcional.
- Tabla de instancias: servidor, motor, BD, FQDN, IP:puerto, usuario, contraseña con botón "ver", estado, fecha.
- Acciones por instancia: **Conectar** (modal con comandos `mysql`/`psql` listos para copy-paste y datos para DBeaver), **Logs** (pipeline completo de provisioning), **Eliminar**.

---

## 5. Configuraciones realizadas (paso a paso)

### 5.1 Preparación del host Windows

- VirtualBox 7.2.6 instalado, `VBoxManage.exe` en `C:\Program Files\Oracle\VirtualBox\`.
- Adaptador Host-Only **VirtualBox Host-Only Ethernet Adapter** con IP `192.168.10.1/24`, DHCP **deshabilitado**.
- Go 1.26 instalado para compilar el manager.
- Llave SSH `~/.ssh/id_ed25519` generada con `ssh-keygen -t ed25519` (la pública se baked en las plantillas).

<<IMAGEN: VirtualBox -> File -> Host Network Manager mostrando el adaptador Host-Only con 192.168.10.1>>

### 5.2 Preparación de la VM base `debian-web1`

Reutilizada del parcial anterior. Debian 13 CLI con: usuario `orlay` con sudo, OpenSSH server, `apache2`, `unzip`. Disco `debian-web1.vdi` convertido a multi-attach:

```bash
VBoxManage modifymedium disk "C:\Users\Orlay Molina\VirtualBox VMs\debian-web1.vdi" --type multiattach
```

<<IMAGEN: VirtualBox -> Tools -> Media mostrando debian-web1.vdi con tipo "Multi-attach">>

### 5.3 Compilación del manager

```bash
cd "C:\Users\Orlay Molina\Documents\Workspace\cloud_computing\httpaas-manager"
go build -o httpaas-manager.exe .
```

<<IMAGEN: Terminal mostrando el binario httpaas-manager.exe recién compilado (ls -la)>>

### 5.4 Generación de las plantillas DBaaS (una sola vez por motor)

El subcomando `--prepare-mariadb-template` (y su equivalente para postgres) hace de manera automática:

1. **Clona** `debian-web1.vdi` → `debian-mariadb.vdi` con `VBoxManage clonemedium`.
2. **Crea una VM temporal** `dbaas-prep-mariadb` con NIC1 Host-Only + NIC2 NAT, attacha el VDI como disco Normal y la inicia headless.
3. **Espera SSH** en `192.168.10.20` (la IP fija del template original).
4. **Fuerza enumeración PCI** dentro del guest con `echo 1 > /sys/bus/pci/rescan` (workaround para un bug intermitente de VBox 7.2 + PIIX3 + Debian 13).
5. **Levanta la NIC NAT** (intenta dhcpcd → ifup → fallback estático `10.0.2.15/24` vía gw `10.0.2.2` con DNS `10.0.2.3` que son los defaults del NAT de VirtualBox).
6. **`apt install`** del motor + configuración (bind-address 0.0.0.0 / listen_addresses '*'), creación del superuser remoto con la contraseña baked-in, instalación de la pubkey SSH del manager.
7. **Apaga** la VM, detacha el disco, lo convierte a `multiattach` y lo re-attacha a la prep VM (necesario para que VBox asigne un *child UUID* y la base sea aceptable por nuevas VMs).
8. La prep VM queda **registrada y apagada** indefinidamente, como dueña del multi-attach.

```bash
./httpaas-manager.exe --prepare-mariadb-template
./httpaas-manager.exe --prepare-postgres-template
```

Cada comando demora ~5-10 min (clon de VDI + apt install + finalización).

<<IMAGEN: Terminal mostrando el output del prep (Cloning ... Creating prep VM ... Running install ... Template ready)>>
<<IMAGEN: VirtualBox -> Tools -> Media mostrando debian-mariadb.vdi y debian-postgres.vdi como Multi-attach>>
<<IMAGEN: VirtualBox -> VM list mostrando dbaas-prep-mariadb y dbaas-prep-postgres registradas en estado Powered Off>>

### 5.5 Variables de entorno (opcional, todas tienen default razonable)

```env
DOMAIN=cloud.local
DNS_AUTH_IP=192.168.10.10
TEMPLATE_INITIAL_IP=192.168.10.20
IP_PREFIX=192.168.10.
GUEST_SSH_USER=orlay
GUEST_SSH_PASSWORD=1234567
GUEST_SSH_KEY=/home/orlay/.ssh/id_ed25519
BASE_VDI_PATH=C:\Users\Orlay Molina\VirtualBox VMs\debian-web1.vdi
MARIADB_BASE_VDI=C:\Users\Orlay Molina\VirtualBox VMs\debian-mariadb.vdi
POSTGRES_BASE_VDI=C:\Users\Orlay Molina\VirtualBox VMs\debian-postgres.vdi
DB_ROOT_PASSWORD=root123
```

### 5.6 Arranque del orquestador

```bash
./httpaas-manager.exe
```

Salida esperada:
```
HTTPaaS + DBaaS Manager iniciado en http://0.0.0.0:8080 (cloud cloud.local)
VBoxManage mode: local (C:\Program Files\Oracle\VirtualBox\VBoxManage.exe)
```

<<IMAGEN: Terminal mostrando el banner del manager arrancado>>

---

## 6. Evidencia del despliegue del servicio

### 6.1 Acceso al dashboard

Navegador en `http://localhost:8080/dbaas.html`. Se ve el formulario de creación y la tabla (vacía al inicio).

<<IMAGEN: Dashboard DBaaS recién abierto, sin instancias>>

### 6.2 Creación de una base MariaDB con SQL inicial

Formulario:
- Motor: **MariaDB**
- Nombre del servidor (VM): `ventas`
- Nombre de la base de datos: `banco`
- Usuario: `admin`
- Contraseña: `12345`
- Script SQL inicial: archivo **`banco.sql`** (creación de tablas `clientes`, `cuentas`, `transacciones`, `creditos` + inserts)

El backend responde `202 Accepted` con el placeholder en estado `creating`.

<<IMAGEN: Dashboard mostrando el formulario lleno antes de "Crear base de datos">>
<<IMAGEN: Dashboard con la nueva instancia en estado "creando" (badge azul pulsando)>>

### 6.3 Aprovisionamiento automático de la VM

El backend invoca `VBoxManage createvm` + `storageattach` con el disco multiconexión `debian-mariadb.vdi`. La nueva VM `ventas-mdb` aparece en VirtualBox y arranca headless.

<<IMAGEN: VirtualBox mostrando ventas-mdb en estado Running con el disco diferencial sobre debian-mariadb.vdi>>

### 6.4 Reconfiguración de hostname e IP estática

Una vez accesible por SSH en la IP de plantilla (`.20`), el orquestador ejecuta el script de configuración: cambia hostname a `ventas.mdb.cloud.local`, asigna IP definitiva `192.168.10.30` y persiste la configuración en `/etc/network/interfaces`. Luego elimina la IP alias `.20` desde una sesión SSH posterior a la nueva IP (paso "dropping initial IP alias" en el log).

<<IMAGEN: Logs de la instancia mostrando los pasos del pipeline (modal de Logs)>>

### 6.5 Registro DNS dinámico

El orquestador se conecta por SSH al ns1 y ejecuta `nsupdate` con la clave `rndc.key`. La zona `cloud.local` se actualiza:

```
ventas.mdb.cloud.local. 60 IN A 192.168.10.30
```

### 6.6 Creación de la BD, usuario y ejecución del .sql

`ConfigureMariaDB` crea la base `banco`, el usuario `admin@'%'` IDENTIFIED BY '12345' con `GRANT ALL PRIVILEGES ON \`banco\`.*` (privilegios scoped, no SUPERUSER). Luego sube el `banco.sql` por SFTP y lo ejecuta como `admin` contra `banco` para probar end-to-end las credenciales.

### 6.7 Verificación desde DBeaver / línea de comandos

Datos de conexión visibles en el modal **Conectar** del dashboard:

```
Host:     192.168.10.30   (o ventas.mdb.cloud.local si DNS apunta a 192.168.10.10)
Port:     3306
Database: banco
User:     admin
Password: 12345
```

<<IMAGEN: Modal "Conectar" del dashboard con los comandos mysql/psql listos para copiar>>
<<IMAGEN: DBeaver conectado a 192.168.10.30:3306 mostrando las tablas de banco>>
<<IMAGEN: Terminal ejecutando: ssh orlay@192.168.10.30 "mysql -h 127.0.0.1 -uadmin -p12345 banco -e 'SELECT * FROM clientes LIMIT 3;'" con resultados>>

### 6.8 Creación de una base PostgreSQL análoga

Repetir el flujo con motor **PostgreSQL**, archivo `muebleria.sql`:

```
Host:     192.168.10.31   (o muebles.pg.cloud.local)
Port:     5432
Database: muebleria
User:     admin2
Password: 12345
```

<<IMAGEN: Dashboard con dos instancias: ventas (MariaDB) y muebles (PostgreSQL), ambas activas>>
<<IMAGEN: psql conectado: PGPASSWORD='12345' psql -h 192.168.10.31 -U admin2 -d muebleria -c "\dt">>

### 6.9 Eliminación de una instancia

Botón **Eliminar** del dashboard → el manager apaga la VM, ejecuta `nsupdate` para borrar el A-record, desconecta el disco multi-attach, ejecuta `unregistervm` y borra el VDI diferencial. El multi-attach base (propiedad de la prep VM) sobrevive intacto.

<<IMAGEN: Dashboard después de eliminar la instancia (tabla queda con una sola fila)>>

---

## 7. Problemas encontrados

A lo largo de la implementación aparecieron varios obstáculos no triviales que no son evidentes en la documentación de VirtualBox/Debian:

### 7.1 Multi-attach requiere VM dueña con child UUID

Al convertir un VDI nuevo a multi-attach con `modifymedium --type multiattach` sin tenerlo adjuntado a ninguna VM, las futuras invocaciones de `storageattach` sobre VMs frescas fallan con `VBOX_E_INVALID_OBJECT_STATE` y el mensaje engañoso *"the media type 'MultiAttach' can only be attached to machines that were created with VirtualBox 4.0 or later"*.

**Resolución:** después de convertir el VDI a multi-attach, se re-attacha inmediatamente a una VM "dueña" (la prep VM en nuestro caso), lo cual hace que VirtualBox cree el primer differencing image y asigne un *child UUID* al base. Sin esa dueña, la base queda en estado "huérfano" inutilizable.

### 7.2 VBox 7.2 + PIIX3 + Debian 13 no siempre enumera NIC2 al boot

A pesar de que `VBoxManage showvminfo` confirma que `nic2=nat` está habilitada y `cableconnected2=on`, el kernel del guest **solo enumera el primer Ethernet controller** en el bus PCI. La segunda NIC existe a nivel de hipervisor pero nunca aparece como dispositivo `enp0s8` en `/sys/class/net/`. Esto rompe la fase de `apt install` durante la preparación de plantillas, porque no hay manera de obtener internet.

**Resolución:** forzar una re-enumeración del bus PCI desde el guest con `echo 1 > /sys/bus/pci/rescan`. Tras eso, el dispositivo aparece como `enp0s8` y se le puede asignar IP (dhcpcd o estática).

### 7.3 Cache de credenciales `sudo` no confiable en sesiones SSH sin TTY

El patrón "primear con `sudo -S -v` y después usar `sudo` sin password" — que funciona en sesiones interactivas — falla **intermitentemente** en sesiones SSH no-interactivas. La quinta invocación de `sudo` reporta *"a terminal is required to read the password"* aunque las cuatro anteriores hayan funcionado. Esto hacía abortar los scripts de configuración de DB justo al final, dejando bases creadas sin los `GRANT` aplicados.

**Resolución:** patrón `SudoRunScript`: el script entero se escribe a `/tmp/dbaas-run-$$.sh` como el usuario SSH (sin sudo, /tmp es world-writable), y se ejecuta en una **única** invocación `echo PASS | sudo -S bash <script>`. El cuerpo del script corre como root sin sudos anidados, evitando el problema del cache.

### 7.4 IP alias `.20` persistente después de ConfigureClone

El script original de `ConfigureClone` agregaba la nueva IP estática a `enp0s3` y dejaba en background un `(sleep 5 && ip addr del <initial-ip>) &` para limpiar la IP de plantilla. Ese background job era **eliminado por systemd** al cerrar la sesión SSH, dejando la VM provisionada respondiendo en **ambas** IPs (la final y la `.20`). Cuando se aprovisionaba una segunda instancia, su SSH a `.20` caía en la primera VM (que tenía el alias residual), no en la nueva, y ConfigureClone modificaba el hostname/IP de la VM equivocada.

**Resolución:** se eliminó el background job y se añadió una nueva función `DropInitialIPAlias` que el handler invoca desde la nueva IP, después de confirmar SSH allí. Limpieza explícita, sincrónica, garantizada.

### 7.5 Carrera `output vacío` del cliente Go SSH

`golang.org/x/crypto/ssh` v0.50 ocasionalmente reporta `Run()` exit 0 con buffer vacío cuando la sesión y el comando duran menos de 1 segundo. Esto enmascaró durante horas el verdadero error del pipeline (que estaba en otra parte) porque parecía un fallo del script remoto.

**Resolución:** documentado en comentarios del código; no es fácil de mitigar sin parchear la librería. En la práctica, se aceptan retries en el debugger y se prioriza el código de salida sobre el buffer.

---

## 8. Propuestas de mejora

- **Persistencia en SQLite** en lugar de `db_instancias.json` para soportar concurrencia real, queries indexadas y rollback transaccional al fallar un provisioning a la mitad.
- **Health-checks periódicos** por instancia (TCP probe a 3306/5432 + ping SQL `SELECT 1`) que reflejen el estado real en el dashboard, no solo el último estado conocido por el manager.
- **Quotas por usuario** y autenticación en el dashboard (OAuth o Basic Auth contra LDAP) — el enunciado lo menciona como opcional.
- **Logs estructurados (JSON)** exportables a Loki/ELK para diagnóstico remoto sin SSH.
- **Métricas Prometheus**: número de instancias por motor, tiempo medio de provisioning, error rate del pipeline, tamaño de las VDIs differencing.
- **Backup automático** de las BDs (mysqldump / pg_dump) hacia un volumen externo o S3-compatible.
- **Migración a virtio-net** en lugar de e1000 para evitar el bug de enumeración PCI (el driver virtio no presenta el problema).
- **Versionado de plantillas**: cada plantilla con un manifiesto de versión y el manager rechaza usar plantillas obsoletas.

---

## 9. Oportunidades de trabajo futuro

- **Soporte de más motores** (MongoDB, Redis, MySQL 8.x) con el mismo patrón de plantilla multi-attach + script de inicialización engine-specific.
- **Restauración desde dump**: en lugar de un `.sql` con DDL+INSERTs, aceptar `mysqldump` o `pg_dump` y restaurarlo en la instancia recién creada — útil para migrar bases existentes.
- **Snapshots y rollback**: aprovechar las VirtualBox snapshots para hacer "checkpoints" antes de operaciones destructivas y permitir restaurar con un clic desde el dashboard.
- **Replicación master-slave** automatizada: el manager puede provisionar pares de VMs configurados como replicación nativa MariaDB o Postgres streaming replication.
- **Migrar el dashboard a SPA** (React/Svelte) con WebSockets para refresco en tiempo real, en lugar de polling cada 4 segundos.
- **Multi-host VirtualBox**: extender el manager para distribuir VMs entre varios hosts físicos, con un scheduler simple por capacidad libre.
- **Empaquetar el manager como un servicio de Windows** (`nssm` o servicio nativo Go) para que arranque con el host.

---

## 10. Conocimientos relevantes aprendidos

- **VirtualBox multi-attach** no es un simple "share disk": requiere una VM dueña que mantenga la referencia (child UUID); sin ella, el medium queda en estado *huérfano* aunque `showmediuminfo` reporte `Type: multiattach`. La documentación oficial no enfatiza esto y el mensaje de error es muy engañoso.
- **El bus PCI virtual de VBox no se actualiza siempre al boot**: cuando hay hot-plug efectivo o NICs agregadas vía `modifyvm`, una re-enumeración manual (`echo 1 > /sys/bus/pci/rescan`) puede ser necesaria.
- **`sudo` en sesiones SSH no-TTY tiene comportamiento inconsistente con cache**: el patrón "prelude + reuse cache" que funciona perfecto en SSH interactivo se rompe intermitentemente en sesiones no-interactivas. La solución robusta es siempre usar `sudo -S` con password en la **misma invocación**, idealmente envolviendo todo el script con un único `sudo bash`.
- **Background jobs en SSH se reaparán por systemd**: `(cmd &) ; disown` no es suficiente — systemd-logind mata todos los procesos asociados a la sesión cuando esta cierra. Para tareas asíncronas reales, hay que usar `systemd-run` o `nohup setsid` con `<dev/null` y output a archivos.
- **El cliente Go SSH `crypto/ssh` v0.50 tiene una race**: comandos cortos a veces devuelven buffer vacío con exit 0. No es un problema funcional (el comando remoto sí corre) pero complica el debugging.
- **Multipart vs JSON**: para uploads, multipart/form-data es lo idiomático en HTTP. Mezclar archivos con campos en JSON requiere base64 y es engorroso — el patrón multipart del primer parcial se transfirió sin fricción al DBaaS.
- **Principio de menor privilegio en bases de datos**: crear roles con `GRANT ALL ON <db>.*` (MariaDB) o `OWNER <db>` (PostgreSQL) en vez de `SUPERUSER` global. El usuario obtiene control total sobre su base sin poder afectar a otros tenants.
- **Idempotencia en la preparación**: scripts de prep que arrancan validando si el VDI destino ya existe; scripts SQL de instancia con `DROP DATABASE IF EXISTS` + `CREATE` para que un re-provision sea seguro.
- **Validación en bordes del sistema**: validar `host_name`, `db_name`, `db_user` a `[a-z0-9_-]` y `db_password` a un set seguro en el handler HTTP, antes de inyectarlo en SQL o scripts shell. Defense-in-depth complementario al escapado.
- **VBox CLI vs GUI**: muchas operaciones que en la GUI son "un botón" requieren múltiples `VBoxManage` (createvm + modifyvm + storagectl + storageattach + startvm). Encapsularlas en helpers `CreateVMFromBase`/`CreateVMWithDirectDisk` paga muchísimo en mantenibilidad.

---

## Anexos

- **README.md** — documentación operativa (env vars, comandos, layout) para futuros mantenedores.
- **banco.sql**, **muebleria.sql** — esquemas demo (banca, mueblería) compatibles con MariaDB y PostgreSQL, listos para subir desde el dashboard.
- **Repositorio del código:** `httpaas-manager/` con las ramas: `main` (HTTPaaS estable) y `feat/db-manage` (DBaaS).
