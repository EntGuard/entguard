FROM golang:1.26 AS migrate

RUN CGO_ENABLED=0 go install github.com/rubenv/sql-migrate/sql-migrate@v1.7.2
COPY db /db

FROM scratch

# Links the published package to this repository, so GHCR shows its source,
# README and license.
LABEL org.opencontainers.image.source="https://github.com/EntGuard/entguard" \
      org.opencontainers.image.title="EntGuard DB Migrate" \
      org.opencontainers.image.description="Database schema migration runner for the EntGuard Orchestrator" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=migrate /go/bin/sql-migrate /bin/sql-migrate
COPY --from=migrate ./db/migrations migrations

ENTRYPOINT ["sql-migrate"]
