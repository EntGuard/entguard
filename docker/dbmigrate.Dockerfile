FROM golang:1.26 AS migrate

RUN CGO_ENABLED=0 go install github.com/rubenv/sql-migrate/sql-migrate@v1.7.2
COPY db /db

FROM scratch

COPY --from=migrate /go/bin/sql-migrate /bin/sql-migrate
COPY --from=migrate ./db/migrations migrations

ENTRYPOINT ["sql-migrate"]
