FROM golang:1.26

RUN apt-get update && apt-get install --no-install-recommends -y \
    unzip \
    wget \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /bin/bash docker
USER docker

ENV HOME=/home/docker
WORKDIR ${HOME}

ENV LOCAL=${HOME}/.local
RUN mkdir -p ${LOCAL}/bin
ENV PATH=$PATH:${LOCAL}/bin

ARG PROTOC_VERSION=27.3
ARG PROTOC_GEN_GO_VERSION=1.34.2
ARG PROTOC_GEN_GO_GRPC_VERSION=1.5.1

RUN wget https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip \
    && unzip protoc-${PROTOC_VERSION}-linux-x86_64.zip -d ${LOCAL}/protoc \
    && rm protoc-${PROTOC_VERSION}-linux-x86_64.zip
ENV PATH=$PATH:${LOCAL}/protoc/bin

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v${PROTOC_GEN_GO_VERSION}
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v${PROTOC_GEN_GO_GRPC_VERSION}