FROM golang:1.26

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN make egserver

CMD ["vpn_server"]
