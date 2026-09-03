ARG DEV_IMAGE="eg-orchestrator-dev:1.14.1"
FROM ${DEV_IMAGE} AS base

FROM scratch

COPY --from=base /go/bin/orchestrator /
COPY --from=base /src/management-ui/dist /management-ui/dist
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

CMD ["/orchestrator"]
