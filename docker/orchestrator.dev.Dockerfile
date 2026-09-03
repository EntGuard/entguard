# syntax=docker/dockerfile:1.7-labs
FROM node:20 AS frontend

WORKDIR /src

COPY ./management-ui/package.json ./management-ui/package-lock.json ./
RUN npm install
COPY ./management-ui ./
RUN npm run build

FROM golang:1.26

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY --exclude=./management-ui . ./
RUN make orchestrator

COPY --from=frontend /src/dist ./management-ui/dist

CMD ["orchestrator"]
