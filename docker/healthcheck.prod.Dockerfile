ARG DEV_IMAGE="eg-healthcheck-dev:1.14.1"
FROM ${DEV_IMAGE} AS base

FROM scratch

COPY --from=base /go/bin/healthcheck /
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

CMD ["/healthcheck"]
