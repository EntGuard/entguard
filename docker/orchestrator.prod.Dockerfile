ARG DEV_IMAGE="eg-orchestrator-dev:1.14.1"
FROM ${DEV_IMAGE} AS base

FROM scratch

# Links the published package to this repository, so GHCR shows its source,
# README and license. Package visibility is set separately, per package.
LABEL org.opencontainers.image.source="https://github.com/EntGuard/entguard" \
      org.opencontainers.image.title="EntGuard Orchestrator" \
      org.opencontainers.image.description="Central management service and web UI for the EntGuard VPN platform" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=base /go/bin/orchestrator /
COPY --from=base /src/management-ui/dist /management-ui/dist
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

CMD ["/orchestrator"]
