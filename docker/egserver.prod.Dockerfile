ARG DEV_IMAGE="ghcr.io/entguard/eg-server-dev:1.14.1"
FROM ${DEV_IMAGE} AS base

FROM scratch

# Links the published package to this repository, so GHCR shows its source,
# README and license.
LABEL org.opencontainers.image.source="https://github.com/EntGuard/entguard" \
      org.opencontainers.image.title="EntGuard Server" \
      org.opencontainers.image.description="WireGuard VPN server node managed by the EntGuard Orchestrator" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=base /go/bin/vpn_server /
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

CMD ["/vpn_server"]
