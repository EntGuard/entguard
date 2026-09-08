# EntGuard __VERSION__ - deployment bundle

This bundle deploys the EntGuard orchestrator stack from the container images
published at `__REGISTRY__`. The full documentation is in
`entguard-__VERSION__-guide.pdf` (also included as `entguard-guide.md`).

## Contents

| Path | Purpose |
|---|---|
| `compose.yaml` | Orchestrator, Postgres, migrations, Telegraf, Prometheus, Grafana |
| `.env` | Pins the image registry and version this bundle was built for |
| `dbconfig.yaml` | Database migration configuration |
| `grafana/`, `prometheus/`, `telegraf/` | Monitoring configuration mounted by compose |
| `generate-dev-certs.sh` | Generates **self-signed** certificates, for evaluation only |

## Prerequisites

Docker Engine with the Compose plugin. The images are pulled from
`__REGISTRY__`, so the host needs network access to that registry - or use the
offline path described at the bottom.

## 1. Generate the database encryption key

EntGuard encrypts sensitive database fields (private keys, TOTP secrets,
WireGuard pre-shared keys, LDAP bind passwords) with AES-GCM. This bundle does
**not** ship a key - generate your own before first start:

```bash
mkdir -p db-encryption-key
openssl rand -out ./db-encryption-key/db-encryption-key.key 16
```

Use `16` for AES-128, `24` for AES-192 or `32` for AES-256. Keep this file
safe: losing it means losing access to every encrypted field. To rotate it
later, use `egvpn crypt --mode=reencrypt` (see the guide).

## 2. Provide TLS certificates

For evaluation you can generate self-signed certificates:

```bash
CERT_DIR=./dev-certs ./generate-dev-certs.sh
```

> **Do not use self-signed certificates for production deployments.** Point the
> `source:` paths of the certificate bind mounts in `compose.yaml` at your own
> CA-signed certificates instead, as described in the guide.

## 3. Start the stack

```bash
docker compose up -d
```

The management UI is then available on port 8080, Grafana on port 3000.

## 4. Install the egvpn client

`egvpn` manages EntGuard VPN server nodes. Pick the binary for your platform,
verify it, and put it on your PATH:

```bash
sha256sum --check --ignore-missing SHA256SUMS
install -m 0755 egvpn-linux-amd64 /usr/local/bin/egvpn
egvpn version
```

This build of `egvpn` starts the server and healthcheck containers from
`__REGISTRY__`. It does not pull them itself, so fetch them first:

```bash
docker pull __REGISTRY__/eg-server:__VERSION__
docker pull __REGISTRY__/eg-healthcheck:__VERSION__
```

## Offline / air-gapped installation

If the VPN node cannot reach the registry, use the image archives shipped
alongside this bundle:

```bash
egvpn install --archive eg-server-__VERSION__.tar.gz
egvpn install --archive eg-healthcheck-__VERSION__.tar.gz
```
