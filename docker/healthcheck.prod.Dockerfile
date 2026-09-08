ARG DEV_IMAGE="eg-healthcheck-dev:1.14.1"
FROM ${DEV_IMAGE} AS base

FROM scratch

# Links the published package to this repository, so GHCR shows its source,
# README and license. Package visibility is set separately, per package.
LABEL org.opencontainers.image.source="https://github.com/EntGuard/entguard" \
      org.opencontainers.image.title="EntGuard Healthcheck" \
      org.opencontainers.image.description="Connectivity healthcheck service for EntGuard VPN server nodes" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=base /go/bin/healthcheck /
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

CMD ["/healthcheck"]
