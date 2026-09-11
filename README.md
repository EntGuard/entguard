# EntGuard

**Enterprise VPN made simple — protect your company and employees at the
same time.**

EntGuard is an orchestrator for WireGuard VPN tunnels. It coordinates VPN
gateways and clients across your organization, automatically generating
keys, distributing configuration, and keeping every peer in sync through a
simple web-based interface — so you get the speed and state-of-the-art
security of WireGuard without ever hand-editing a config file.

Cloud-native and cross-platform (Windows, Linux, Android), EntGuard gives
organizations centralized control over access and security policy, while
keeping remote access effortless for every employee.

---

This is a documentation for developers. It serves these purposes:
- A quick start guide for developers.
- To explain things that are important for developers but are not needed for users.

You can find the full (user-oriented) documentation here: [docs/entguard-guide.md](docs/entguard-guide.md).

## Installation

### Prerequisites

* [Go 1.26+](https://go.dev/doc/install)
* [Docker](https://docs.docker.com/engine/install/) (minimal version 24.0.0)
* [Docker Compose](https://docs.docker.com/compose/install/) (minimal version 2.20.0)

You may be required to preface each `docker` command in this guide with `sudo`. If you want to avoid this,
follow the steps in the [Docker post-installation guide](https://docs.docker.com/engine/install/linux-postinstall/).

### Set up secrets 

1. Prepare TLS certificates to provide communications security in the gRPC connections.

   For testing you can generate self-signed certificates:

   ```bash
   make dev-certs
   ```

   > **NOTE:** Do not use self-signed certificates for production deployments.

2. Generate a database encryption key

   ```bash
   mkdir -p db-encryption-key
   openssl rand -out ./db-encryption-key/db-encryption-key.key 16
   ```

### Build components

Build all EntGuard components (orchestrator, server, healthcheck, egvpn tool, and database migrations):

```bash
make all
```

Or build components individually:

- Build docker images (development and production) with the EntGuard Orchestrator:

  ```bash
  make orchestrator-images
  ```

- Build docker images (development and production) with the EntGuard Server:

  ```bash
  make egserver-images
  ```

- Build docker images (development and production) with the EntGuard Healthcheck:

  ```bash
  make healthcheck-images
  ```

- Build the `egvpn` tool which helps to run the EntGuard Server:

  ```bash
  make egvpn
  ```

- Build docker image with the database migrations:

  ```bash
  make dbmigrate-image
  ```

## How to run the EntGuard Orchestrator

Go to the `provisioning/docker` directory:

```bash
cd ./provisioning/docker
```

*Optional:* Enable debug logs. In the `compose.yaml` file, find
`"VPN_ORCHESTRATOR_LOG_LEVEL"` and change it from `"info"` to `"debug"`.

Create and start containers:

```bash
docker compose up --detach
```

> **NOTE:** For a PostgreSQL database a volume is used to store data. If the
> volume does not exist, it will be initialized. Please see the notes at the
> bottom of [Updating EG-O](docs/entguard-guide.md#updating-eg-o) for how the
> volume and optional DB dump are used. The internal functionality is explained
> in [Postgres Docker image documentation](https://hub.docker.com/_/postgres/),
> section Initialization scripts.

After that, you may open <https://127.0.0.1:8080/> in your browser and login as the default
admin user:

* username: `admin`
* password: `5Bt3kp0ItQ;9`

To stop the Orchestrator use:

```bash
docker compose down
```

If you also want to remove the volume used to store PostgreSQL data use:

```bash
docker compose down --volumes
```

## How to run the EntGuard Server and EntGuard Healthcheck service

First, start the Orchestrator and create a server with some name (e.g. kyiv).
Set the field *Endpoint* according to where the server will de deployed.
Set the field *Healthcheck Address* according to where the healthcheck service
will de deployed, or leave it empty to not use the healthcheck service.

> **NOTE:** If you set the *Healthcheck Address*, then the healthcheck ping is
mandatory for EntGuard clients. If the Healthcheck service is not running,
clients will report connection issues and will automatically disconnect.

> **NOTE 2:** When you test the deployment locally, it may be tempting to set
> the *Healthcheck Address* to the same value as the server *Endpoint*, to
> simplify the local deployment. Don't do it. If you do it, then the healthcheck
> address (which in this case is also the server endpoint) gets automatcally
> added into client side allowed IPs → on connecting, the address gets added to
> the host’s routing table to be routed via eg0 (virtual) interface → after the
> eg0 interface encrypts the packets, the host tries to physically reach the
> server again via the virtual eg0 interface, hence it fails (after encrypting
> packets at eg0, the host must use physical interface so the encrypted packets
> can physically reach the server).
> 
> The solution is to not use the server endpoint as the healthcheck address.  
>
> When someone is deploying EntGuard in production and follows the user
> documentation, it says that the healthcheck address must not be
> publicly reachable (unlike the server endpoint), so this issue will not
> happen.

After that, use `egvpn` to prepare the configuration:

```bash
sudo $GOPATH/bin/egvpn config init --server-name kyiv
```
*Optional:* To enable debug logs, invoke the previous command with the flag `--log-level debug`
```bash
sudo $GOPATH/bin/egvpn config init --server-name kyiv --log-level debug
```

You will be prompted for a username and a password of the admin user. The default admin user login credentials are:

* username: `admin`
* password: `5Bt3kp0ItQ;9`

As a result, three files will be created:

* `$PWD/tls-auth.json` with the TLS certificate for authentication
* `$PWD/config.json` with the configuration for the server
* `/etc/entguard/egvpn-config.json` with the configuration for `egvpn`

Now you can start the server and the healthcheck service
(replace `<component>` with one of `server`, `healthcheck`, `all`):

```bash
sudo $GOPATH/bin/egvpn start -s <component>
```

As a result, the server and/or healthcheck service will be started and one file will be created:

* `/etc/entguard/egvpn-restart-opts.json` with the restart options for `egvpn`

> **NOTE:** To start the server, WireGuard should be installed.

To stop the server and/or healthcheck service use:

```bash
$GOPATH/bin/egvpn stop -s <component>
```

> **NOTE:** If you have set the `$GOBIN` env variable to something else than `$GOPATH/bin`, you may need to replace
> `$GOPATH/bin` with `$GOBIN` in the `egvpn` commands.

### VPP
The instructions above use the Linux WireGuard implementation. For better performance, 
the [VPP](https://wiki.fd.io/view/VPP) can be used instead of the Linux. This completely skips the processing of 
the packets in the linux network stack and uses user-space VPP application to handle the packets.

The setup is similar to Linux WireGuard usage (using the same `egvpn` tool, behave the same way for 
non-VPP-related parts), but has additional complexity 
(check [user documentation](docs/entguard-guide.md#iii-entguard-server-eg-s) for the full user guide):
1. Check [additional requirements](docs/entguard-guide.md#prerequisites-1) from the user documentation
2. Create the initial config files as for Linux WireGuard and add some additional parameters:
   ```bash
   sudo $GOPATH/bin/egvpn config init --server-name kyiv --use-vpp --vpp-interfaces="<network device PCI addresses>"
   ```
   The `--use-vpp` switch will enable the creation of the additional VPP configuration files:
   * `$PWD/vpp.config` with configuration for the VPP
   * `$PWD/vpp-day0.config` with [VPP CLI][vpp-cli] topology setup 
     configuration for the VPP (is applied by `egvpn` when VPP starts)

   The `--vpp-interfaces` flag is a comma-separated list of network device PCI addresses (i.e.
   `"0000:00:09.0,0000:00:0a.0"`) that should be dedicated to the VPP. To find out the PCI address of your network 
   device use i.e. `sudo lshw -class network` where in output the logical name is interface name and the bus info 
   contains the PCI address.

   The default zero day configuration file content is example of using 2 dedicated network devices/interfaces
   for VPP. The example brings these interfaces(dpdk1,dpdk2) up, assign IP addresses to them and tags one
   of them as the outside interface for connecting to EntGuard clients (EG-C). The other interface is meant
   for connecting EntGuard Server (EG-S) with internal network that contain sites/resources that need the VPN
   protection of EntGuard. This is just an example of possible usage. Many other scenarios are possible.
   Feel free to explore the possibilities by checking out the [VPP CLI][vpp-cli].

   *Before starting server, please review/change your generated zero day configuration file to reflect 
   your network topology!*

   For example if you have only 2 OS interfaces and therefore can spare/dedicate only one interface to VPP, then 
   you can i.e. comment out the second interface `dpdk2` in default zero day config (ip address and state setting).

   The DPDK in the VPP needs physical network devices(or virtualized that are pretending to be physical, like interfaces in VM)
   and that could be problematic for development or testing. It is possible to replace those DPDK interfaces with some
   virtual interfaces that will serve the same purpose (with different performance though -> do not use for 
   performance testing). To do this just don't use the `--vpp-interfaces` flag and define with VPP CLI in 
   `vpp-day0.config` some other interfaces instead. You can create 
   [veth tunnel example](https://fd.io/docs/vpp/v2101/gettingstarted/progressivevpp/interface) or create tap tunnel.
   For tap tunnel use [create tap command](https://s3-docs.fd.io/vpp/24.06/cli-reference/clis/clicmd_src_vnet_devices_tap.html#create-tap), 
   set ip address and state for both ends (the VPP end and the linux end that is not in the host but in VPP 
   container->can be moved out to host with linux ip tooling).

3. Start the server just like for Linux WireGuard implementation (Warning: 
   *change generated zero day configuration to your topology before server start*):
   ```bash
   sudo $GOPATH/bin/egvpn start
   ```
4. Stop the server just like for Linux WireGuard implementation:
   ```bash
   sudo $GOPATH/bin/egvpn stop
   ```

For troubleshooting check out [VPP troubleshooting](#vpp-troubleshooting).

## EntGuard Client

There are various EntGuard Clients available for various platforms. Please see the user documentation for the list of [EntGuard Clients](docs/entguard-guide.md#v-entguard-client-eg-c).

## Troubleshooting
### VPP Troubleshooting
The basic troubleshooting can be found in the [user documentation](docs/entguard-guide.md#troubleshooting). 
This is the advanced troubleshooting section for VPP/DPDK.

- Enter the interactive VPP console (and use the 
  [VPP CLI][vpp-cli] commands):
  ```bash
  $ docker exec -it eg-s-vpp vppctl
  ```
- Listing routes in the VPP console (check output whether needed routes exist and lead to the correct output interfaces (
  [generic example of routing table output](https://fd.io/docs/vpp/v2101/gettingstarted/progressivevpp/traces#examine-routing-tables))):
  ```
  vpp# show ip fib
  ```
- If everything above looks good(check also the user documentation troubleshooting), but it still fails 
  (in the VPP) then use [VPP packet tracing](https://fd.io/docs/vpp/v2101/gettingstarted/progressivevpp/traces) 
  (Note: tracing always tracks the packets on input processing nodes, for the WireGuard interfaces it is 
  `virtio-input` and for the external DPDK interfaces it is `dpdk-input`)
- Exit VPP console:
  ```
  vpp# quit
  ```

#### Troubleshooting missing (DPDK) interfaces
The system interfaces, that should be dedicated to the VPP, will be handled by [DPDK](https://www.dpdk.org/) tooling.
The DPDK is integrated into the VPP, but to properly use the PCI network devices it also needs that these PCI
network device use DPDK compatible driver. The difference between the PCI network device to be used by the linux kernel
(be visible as interface in the linux) and using it directly by the VPP (bypassing the linux kernel) is just 
usage of a different driver for the network device.

You can find out the driver state for your network device by using DPDK's `dpdk-devbind.py` script. You can
install it i.e. by installing DPDK:
```bash
sudo apt install dpdk
```
You can find out the bounded driver for your network device:
```bash
sudo dpdk-devbind.py -s
```
The correct state of network devices should look something like this (after VPP start):
```
Network devices using DPDK-compatible driver
============================================
0000:00:09.0 '82540EM Gigabit Ethernet Controller 100e' drv=vfio-pci unused=e1000
0000:00:0a.0 '82540EM Gigabit Ethernet Controller 100e' drv=vfio-pci unused=e1000

Network devices using kernel driver
===================================
0000:00:03.0 '82540EM Gigabit Ethernet Controller 100e' if=enp0s3 drv=e1000 unused=vfio-pci *Active*
0000:00:08.0 '82540EM Gigabit Ethernet Controller 100e' if=enp0s8 drv=e1000 unused=vfio-pci *Active*
```
The network devices in your case may have different PCI addresses or you might have different number of network
devices, but the gist is the grouping of the interfaces by the kernel and the DPDK-compatible drivers. 
The `vfio-pci` driver is not the only DPDK-compatible driver, but it is the preferred option (for more options 
see notes from [DPDK documentation](https://doc.dpdk.org/guides/linux_gsg/linux_drivers.html)).

The VPP should update the driver bound to network device when the related linux interface is in the `DOWN` state.
However, you can unbind the current driver, i.e.:
```bash
sudo dpdk-devbind.py -u 0000:00:09.0
```
If that is not enough for the VPP the handle network device automatically, then also bind the correct
driver to the network device, i.e.:
```bash
sudo dpdk-devbind.py -b vfio-pci 0000:00:09.0
```

## EntGuard Orchestrator

**EntGuard Orchestrator** exposes different ports for different types of connection:

| Type  | Purpose                                          |  Default | Environment Variable                     |
|-------|--------------------------------------------------|---------:|------------------------------------------|
| HTTPS | accessing REST API and web UI app                |     8080 | `VPN_ORCHESTRATOR_REST_PORT`             |
| gRPC  | communicating with EntGuard servers              |     8081 | `VPN_ORCHESTRATOR_RPC_PORT_VPN_S`        |
| gRPC  | communicating with EntGuard clients              |     8082 | `VPN_ORCHESTRATOR_RPC_PORT_VPN_C`        |
| gRPC  | communicating with EntGuard Healthcheck services |     8083 | `VPN_ORCHESTRATOR_RPC_PORT_HEALTH_CHECK` |

Additionally, **EntGuard Healthcheck service** exposes a port. This can be
configured via `--hc-listen-port` flag of `egvpn config init`. This port must
also be configured in orchestrator environment variable (the orchestrator needs
to know the port so it can inform the EntGuard clients about it):

| Type | Purpose                                          |  Default | Environment Variable                     |
|------|--------------------------------------------------|---------:|------------------------------------------|
| gRPC | communicating with EntGuard clients              |     8084 | `VPN_HEALTH_CHECK_RPC_PORT_VPN_C`        |

Checkout other possible configurations and their defaults in the [`config.go`][orchestrator-config] file.

All REST API endpoints are prefixed with `/api/v1/`. Accessing the REST API is only available for admin users.
By default, authentication based on JWT is used, but it can be switched to BasicAuth.

> **NOTE:** The web app does not support BasicAuth.

The REST API endpoints are defined in [service/api/v1/rest.go].

The REST API and orchestrator UI requests from users/browser to Orchestrator are encrypted by TLS (HTTPS connection):

- Authentication of orchestrator (TLS):
    - The orchestrator uses TLS certificates to prove its identity to web browser and other users of REST API. It 
      loads the certificates at startup from the paths specified in `compose.yaml` file. For local development and 
      testing it uses self-signed certificates, which are stored in the files `grpc-server-cert.pem` and 
      `grpc-server-key.pem` in the folder `provisioning/docker/dev-certs`.
    - The browser (and so should users also do) uses certificate authority (CA) certificate of the operating system to 
      verify the orchestrator's certificates.

The gRPC API services are defined in [proto/v1/vpncfg.proto] and other files in that folder.

The gRPC requests from EntGuard clients to the EntGuard orchestrator are authenticated with TLS and BasicAuth:

- Authentication of orchestrator (TLS):
  - The orchestrator uses TLS certificates to prove its identity to client. It loads the certificates at startup
    from the paths specified in `compose.yaml` file. For local development and testing it uses self-signed certificates,
    which are stored in the files `grpc-server-cert.pem` and `grpc-server-key.pem`
    in the folder `provisioning/docker/dev-certs`.
  - The client uses certificate authority (CA) certificate of the operating system to verify the orchestrator's certificates.
- Authentication of client (BasicAuth):
  - The client authenticates to the orchestrator using username and password.

The gRPC requests from EntGuard servers to the EntGuard orchestrator are authenticated with mTLS (mutual TLS):

- Authentication of orchestrator (TLS):
  - The orchestrator uses TLS certificates to prove its identity to server (the same certificates
    as for the client <-> orchestrator communication).
  - The server uses CA certificate to verify the orchestrator's certificates. It loads the CA certificate at startup
    from `config.json` file (created by `egvpn config init`). For local development and testing, the egvpn uses
    the CA certificate at `provisioning/docker/dev-certs/rootCA.pem`.
- Preparation for TLS authentication of server:
  - When the orchestrator first starts, it initializes a new certificate provider (CA certificate and CA private key)
    and stores it in database. This new certificate provider is created from random data (it is not related to any
    of the certificates mentioned above).
  - When `egvpn config init` is used to create the file `tls-auth.json`, it authenticates to the orchestrator using
    BasicAuth (username/password). Then the orchestrator creates a new *server tag* for the server and creates
    a certificate for the server tag, signed by the certificate provider stored in database (from the previous point).
    Then it sends the certificate to the egvpn and egvpn saves it to `tls-auth.json`.
- Authentication of server (TLS + custom verification of server tag):
  - The server uses TLS certificates to prove its identity to server. It loads the certificate at startup from `tls-auth.json` file.
  - The orchestrator uses CA certificate stored in the database to verify the server's certificates (this is handled
    by golang gRPC library). Then it verifies the server tag (this is handled by custom implementation, the function
    `TLSAuth` in the file `service/rpc/vpns.go`).
- The mTLS is enabled by the field `ClientAuth: tls.RequireAndVerifyClientCert` in the struct `tls.Config`
(file `service/rpc/server.go`).

### Database

**PostgreSQL database** is required for the **EntGuard Orchestrator**. The database schema is defined
in the migration files located in `db/migrations/postgres`.

### How to create and apply new database migration script

Create a new migration script providing file name with current release version:

```bash
make migrate-new NAME=v1.7.3-new-file-name
```

Open newly created migration file and add sql statements according to the [documentation](https://github.com/rubenv/sql-migrate#writing-migrations).

Update database to the latest version available (executes all migration scripts):
```bash
make migrate-up
```
Migration up is also automitcally performed when starting the deployment using `docker compose up`.

Undo a database migration (rolls back only latest migration script):
- First, make a backup of the database! The migration down may lead to irreversibly losing data that is not supported in the older version.
- Then run:
  ```bash
  make migrate-down
  ```

## EntGuard Server

The EntGuard server is able to configure WireGuard interfaces from the `Configuration` proto message.
That message, as well as all others gRPC messages/services, are defined in [proto/v1/vpncfg.proto].

[orchestrator-config]: service/config/config.go
[service/api/v1/rest.go]: service/api/v1/rest.go
[proto/v1/vpncfg.proto]: proto/v1/vpncfg.proto
[vpp-cli]: https://s3-docs.fd.io/vpp/24.06/cli-reference/index.html
