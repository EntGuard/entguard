COMMIT ?= $(shell git rev-parse HEAD)
BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)

REMOTE_GIT := https://github.com/EntGuard/entguard.git

RELEASE_VERSION="1.15.0"
RELEASE_FILENAME="entguard-release-v$(RELEASE_VERSION)"

ORCHESTRATOR_VERSION=$(RELEASE_VERSION)
ORCHESTRATOR_IMAGE_DEV="eg-orchestrator-dev"
ORCHESTRATOR_IMAGE_PROD="eg-orchestrator"

EGSERVER_VERSION=$(RELEASE_VERSION)
EGSERVER_IMAGE_DEV="eg-server-dev"
EGSERVER_IMAGE_PROD="eg-server"

HEALTHCHECK_VERSION=$(RELEASE_VERSION)
HEALTHCHECK_IMAGE_DEV="eg-healthcheck-dev"
HEALTHCHECK_IMAGE_PROD="eg-healthcheck"

EGVPN_VERSION=$(RELEASE_VERSION)

DBMIGRATE_VERSION=$(RELEASE_VERSION)
DBMIGRATE_IMAGE="eg-db-migrate"

GHCR_IMAGE_REPO_URL="ghcr.io/entguard"

ORCHESTRATOR_LDFLAGS = -w -s \
	-X github.com/entguard/entguard/service/buildinfo.version=$(ORCHESTRATOR_VERSION) \
	-X github.com/entguard/entguard/service/buildinfo.gitCommit=$(COMMIT) \
	-X github.com/entguard/entguard/service/buildinfo.gitBranch=$(BRANCH)

EGVPN_LDFLAGS = -w -s \
	-X github.com/entguard/entguard/cmd/egvpn/buildinfo.version=$(EGVPN_VERSION) \
	-X github.com/entguard/entguard/cmd/egvpn/buildinfo.gitCommit=$(COMMIT) \
	-X github.com/entguard/entguard/cmd/egvpn/buildinfo.gitBranch=$(BRANCH) \
	-X github.com/entguard/entguard/cmd/egvpn/buildinfo.egServerImageName=$(GHCR_IMAGE_REPO_URL)/$(EGSERVER_IMAGE_PROD) \
	-X github.com/entguard/entguard/cmd/egvpn/buildinfo.egServerImageTag=$(EGSERVER_VERSION) \
	-X github.com/entguard/entguard/cmd/egvpn/buildinfo.egHealthcheckImageName=$(GHCR_IMAGE_REPO_URL)/$(HEALTHCHECK_IMAGE_PROD) \
	-X github.com/entguard/entguard/cmd/egvpn/buildinfo.egHealthcheckImageTag=$(HEALTHCHECK_VERSION)

.PHONY: all orchestrator egserver egvpn healthcheck test test-unit test-integration test-clean

all: all-images egvpn

orchestrator:
	@echo "=> installing EntGuard Orchestrator"
	CGO_ENABLED=0 go install -ldflags "${ORCHESTRATOR_LDFLAGS}" ./cmd/orchestrator

egserver:
	@echo "=> installing EntGuard Server"
	CGO_ENABLED=0 go install ./cmd/vpn_server

egvpn:
	@echo "=> installing egvpn"
	CGO_ENABLED=0 go install -ldflags "${EGVPN_LDFLAGS}" ./cmd/egvpn/

healthcheck:
	@echo "=> installing EntGuard Healthcheck service"
	CGO_ENABLED=0 go install ./cmd/healthcheck

# -------------------------------
# Tests
# -------------------------------

test: test-unit test-integration

test-unit:
	@echo "=> running unit tests"
	TEST_UNIT=1 go test ./...

test-integration: test-clean test-integration-cached

test-integration-cached:
	@ echo "=> running integration tests"
	docker compose -f provisioning/docker/compose.test-integration.yaml up -d --build
	TEST_INTEGRATION=1 go test -p=1 `go list ./... | grep -v pkg`
	docker compose -f provisioning/docker/compose.test-integration.yaml down -v

test-fuzz-rest: test-clean test-fuzz-rest-cached

# Absolute: go test runs each package in its own directory.
FUZZ_REST_COMPOSE_FILE:=$(CURDIR)/provisioning/docker/compose.test-fuzz-rest.yaml

test-fuzz-rest-cached:
	@echo "=> running REST API fuzz tests"
	docker compose -f $(FUZZ_REST_COMPOSE_FILE) up -d --build
	TEST_FUZZ_REST=1 TEST_FUZZ_COMPOSE_FILE=$(FUZZ_REST_COMPOSE_FILE) \
		go test -p=1 `go list ./... | grep -v pkg | grep -v cmd`
	docker compose -f $(FUZZ_REST_COMPOSE_FILE) down -v

test-rest-local:
	@echo "=> running REST API tests locally"
	docker compose -f provisioning/docker/compose.test-rest-local-db.yaml up -d --build
	TEST_LOCAL_REST_HANDLERS=1 go test -p=1 ./service/rest
	docker compose -f provisioning/docker/compose.test-rest-local-db.yaml down -v

test-clean:
	go clean -testcache

test-coverage: # excludes _mock.go files
	@ echo "=> analyzing tests coverage"
	@ TEST_UNIT=1 go test ./... -coverprofile=unit.out
	@ cat unit.out | grep -v '_mock.go' | grep -v 'querier.go' | grep -v 'ldap.go' > cover_unit.out
	@ docker compose -f provisioning/docker/compose.test-integration.yaml up -d --build
	@ TEST_INTEGRATION=1 go test -p=1 `go list ./... | grep -v pkg | grep -v cmd` -coverprofile=integration.out
	@ docker compose -f provisioning/docker/compose.test-integration.yaml down -v
	@ cat integration.out | grep -v '_mock.go' > cover_integration.out
	@ hash gocovmerge > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		go install github.com/wadey/gocovmerge; \
		fi
	@ gocovmerge cover_unit.out cover_integration.out > cover.out
	@ echo "=> tests coverage percentage per function:"
	@ go tool cover -func=cover.out
	@ rm *.out

# -------------------------------
# Docker Images
# -------------------------------

.PHONY: all-images orchestrator-images orchestrator-dev-image orchestrator-prod-image \
	egserver-images egserver-dev-image egserver-prod-image \
	healthcheck-images healthcheck-dev-image healthcheck-prod-image \
	dbmigrate-image

all-images: orchestrator-images egserver-images healthcheck-images dbmigrate-image

orchestrator-images: orchestrator-dev-image orchestrator-prod-image

egserver-images: egserver-dev-image egserver-prod-image

healthcheck-images: healthcheck-dev-image healthcheck-prod-image

orchestrator-dev-image:
	@echo "=> building EntGuard Orchestrator development image"
	docker build \
		--file docker/orchestrator.dev.Dockerfile \
		--tag $(GHCR_IMAGE_REPO_URL)/${ORCHESTRATOR_IMAGE_DEV}:${ORCHESTRATOR_VERSION} \
		.

orchestrator-prod-image:
	@echo "=> building EntGuard Orchestrator production image"
	docker build \
		--build-arg DEV_IMAGE=$(GHCR_IMAGE_REPO_URL)/${ORCHESTRATOR_IMAGE_DEV}:${ORCHESTRATOR_VERSION} \
		--file docker/orchestrator.prod.Dockerfile \
		--tag $(GHCR_IMAGE_REPO_URL)/${ORCHESTRATOR_IMAGE_PROD}:${ORCHESTRATOR_VERSION} \
		.

egserver-dev-image:
	@echo "=> building EntGuard Server development image"
	docker build \
		--file docker/egserver.dev.Dockerfile \
		--tag $(GHCR_IMAGE_REPO_URL)/${EGSERVER_IMAGE_DEV}:${EGSERVER_VERSION} \
		.

egserver-prod-image:
	@echo "=> building EntGuard Server production image"
	docker build \
		--build-arg DEV_IMAGE=$(GHCR_IMAGE_REPO_URL)/${EGSERVER_IMAGE_DEV}:${EGSERVER_VERSION} \
		--file docker/egserver.prod.Dockerfile \
		--tag $(GHCR_IMAGE_REPO_URL)/${EGSERVER_IMAGE_PROD}:${EGSERVER_VERSION} \
		.

healthcheck-dev-image:
	@echo "=> building EntGuard Healthcheck development image"
	docker build \
		--file docker/healthcheck.dev.Dockerfile \
		--tag $(GHCR_IMAGE_REPO_URL)/${HEALTHCHECK_IMAGE_DEV}:${HEALTHCHECK_VERSION} \
		.

healthcheck-prod-image:
	@echo "=> building EntGuard Healthcheck production image"
	docker build \
		--build-arg DEV_IMAGE=$(GHCR_IMAGE_REPO_URL)/${HEALTHCHECK_IMAGE_DEV}:${HEALTHCHECK_VERSION} \
		--file docker/healthcheck.prod.Dockerfile \
		--tag $(GHCR_IMAGE_REPO_URL)/${HEALTHCHECK_IMAGE_PROD}:${HEALTHCHECK_VERSION} \
		.

dbmigrate-image:
	@echo "=> building database migration image"
	docker build \
		--file docker/dbmigrate.Dockerfile \
		--tag $(GHCR_IMAGE_REPO_URL)/${DBMIGRATE_IMAGE}:${DBMIGRATE_VERSION} \
		.

protoc-image:
	@echo "=> building protoc image"
	docker build --file docker/protoc.Dockerfile --tag goprotoc:latest .

# -------------------------------
# Development helpers
# -------------------------------

.PHONY:install-linter
install-linter: install-golangci-lint install-buf-linter

.PHONY:lint
lint: golangci-lint buf-lint

GOLANGCI_LINT_VERSION:=2.11.4

.PHONY:install-golangci-lint
install-golangci-lint:
	@echo "=> installing golangci-lint $(GOLANGCI_LINT_VERSION) into $$(go env GOPATH)/bin"
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v$(GOLANGCI_LINT_VERSION)

.PHONY:golangci-lint
golangci-lint:
	@echo "=> detecting if golangci-lint $(GOLANGCI_LINT_VERSION) is installed"
	@golangci-lint version | grep -q $(GOLANGCI_LINT_VERSION) \
		|| (echo "golangci-lint $(GOLANGCI_LINT_VERSION) is not installed. To install it run \`make install-golangci-lint\`" \
			&& exit 1);
	@echo "=> found golangci-lint"
	@echo "=> cleaning golangci-lint cache"
	golangci-lint cache clean
	@echo "=> running golangci-lint"
	golangci-lint run --verbose --timeout 10m

BUF_LINT_VERSION:=1.37.0

.PHONY:install-buf-linter
install-buf-linter:
	@echo "=> installing buf linter $(BUF_LINT_VERSION) into $$(go env GOPATH)/bin"
	go install github.com/bufbuild/buf/cmd/buf@v$(BUF_LINT_VERSION)

.PHONY:buf-lint
buf-lint:
	@echo "=> detecting if buf linter $(BUF_LINT_VERSION) is installed"
	@buf --version | grep -q $(BUF_LINT_VERSION) \
		|| (echo "buf $(BUF_LINT_VERSION) is not installed. To install it run \`make install-buf-linter\`" \
			&& exit 1);
	@echo "=> running protobuf linter"
	buf lint

.PHONY:buf-breaking
buf-breaking:
	@echo "=> detecting if buf linter $(BUF_LINT_VERSION) is installed"
	@buf --version | grep -q $(BUF_LINT_VERSION) \
		|| (echo "buf $(BUF_LINT_VERSION) is not installed. To install it run \`make install-buf-linter\`" \
			&& exit 1);
	@echo "=> running protobuf breaking changes detection"
	buf breaking --against "$(REMOTE_GIT)#branch=main"

.PHONY:dev-certs
dev-certs:
	CERT_DIR=./provisioning/docker/dev-certs ./scripts/generate-dev-certs.sh

PROTO_FILE_PATH = "./proto/v2"

.PHONY:generate-proto
generate-proto:
	@if [ -z "$(shell docker images -q goprotoc:latest 2> /dev/null)" ]; then echo "=> No 'goprotoc' image found. Run 'make protoc-image'" && exit 1 ; fi
	@echo "=> generating proto bindings for '$(PROTO_FILE_PATH)'"
	@docker run --rm --volume $(shell pwd):/home/docker/app --workdir /home/docker/app goprotoc:latest \
	protoc \
		-I . \
		$(PROTO_FILE_PATH)/*.proto \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative
	@echo "=> generated!"

# -------------------------------
# Vulnerability scans
# -------------------------------

.PHONY:vulnerability-scan
vulnerability-scan: govulncheck osv-scanner trivy-scan

GOVULNCHECK_VERSION:=1.3.0
GOVULNCHECK_OUTPUT:=govulncheck-results.txt
GO_VERSION:=1.26.1

.PHONY:govulncheck
govulncheck:
	@echo "=> checking for newer govulncheck version..."
	@LATEST=$$(curl -sf "https://proxy.golang.org/golang.org/x/vuln/@latest" \
		| grep '"Version"' | sed 's/.*"Version": *"v\([^"]*\)".*/\1/'); \
	if [ -n "$$LATEST" ] && [ "$$LATEST" != "$(GOVULNCHECK_VERSION)" ]; then \
		MSG="WARNING: govulncheck v$(GOVULNCHECK_VERSION) is outdated, latest is v$$LATEST. Update GOVULNCHECK_VERSION in Makefile."; \
		echo "$$MSG"; \
		echo "$$MSG" >> $(GOVULNCHECK_OUTPUT); \
	fi
	@echo "=> running govulncheck v$(GOVULNCHECK_VERSION) in Docker, saving results to $(GOVULNCHECK_OUTPUT)"
	@docker run --rm \
		-v $(shell pwd):/app \
		-w /app \
		golang:$(GO_VERSION) \
		sh -c "go install golang.org/x/vuln/cmd/govulncheck@v$(GOVULNCHECK_VERSION) && govulncheck -show verbose ./..." \
		2>&1 | tee $(GOVULNCHECK_OUTPUT)

OSV_SCANNER_VERSION:=2.3.5
OSV_OUTPUT:=osv-results.txt

.PHONY:osv-scanner
osv-scanner:
	@echo "=> checking for newer osv-scanner version..."
	@LATEST=$$(curl -sf https://api.github.com/repos/google/osv-scanner/releases/latest \
		| grep '"tag_name"' | sed 's/.*"tag_name": *"v\([^"]*\)".*/\1/'); \
	if [ -n "$$LATEST" ] && [ "$$LATEST" != "$(OSV_SCANNER_VERSION)" ]; then \
		MSG="WARNING: osv-scanner v$(OSV_SCANNER_VERSION) is outdated, latest is v$$LATEST. Update OSV_SCANNER_VERSION in Makefile."; \
		echo "$$MSG"; \
		echo "$$MSG" >> $(OSV_OUTPUT); \
	fi
	@echo "=> running osv-scanner v$(OSV_SCANNER_VERSION) against go.mod, saving results to $(OSV_OUTPUT)"
	@docker run --rm \
		-v $(shell pwd):/src \
		ghcr.io/google/osv-scanner:v$(OSV_SCANNER_VERSION) \
		--lockfile /src/go.mod \
		2>&1 | tee $(OSV_OUTPUT)

TRIVY_VERSION:=0.69.3
TRIVY_OUTPUT:=trivy-results.txt

.PHONY:trivy-scan
trivy-scan:
	@echo "=> running trivy v$(TRIVY_VERSION), saving results to $(TRIVY_OUTPUT)"
	@TRIVY_VERSION="$(TRIVY_VERSION)" \
		EGSERVER_IMAGE="$(GHCR_IMAGE_REPO_URL)/$(EGSERVER_IMAGE_PROD):$(EGSERVER_VERSION)" \
		HEALTHCHECK_IMAGE="$(GHCR_IMAGE_REPO_URL)/$(HEALTHCHECK_IMAGE_PROD):$(HEALTHCHECK_VERSION)" \
		./scripts/trivy-scan.sh \
		2>&1 | tee $(TRIVY_OUTPUT)

# -------------------------------
# Third-party licenses for *compiled binaries*
# -------------------------------

GO_LICENSES_VERSION:=1.6.0

.PHONY:third-party-licenses
third-party-licenses:
	@echo "=> regenerating directory 'third_party_licenses' for the shipped binaries"
	@go install github.com/google/go-licenses@v$(GO_LICENSES_VERSION)
	@rm -rf third_party_licenses
	@go mod download
	@go-licenses save ./cmd/orchestrator/... ./cmd/vpn_server/... ./cmd/healthcheck/... ./cmd/egvpn/... \
		--save_path=third_party_licenses || true
	@rm -rf third_party_licenses/pantheon.tech
# after invoking `make third-party-licenses`, read the terminal output and
# update the following section as necessary
	@mkdir -p third_party_licenses/github.com/dgryski/dgoogauth
	@echo "github.com/dgryski/dgoogauth does not ship a LICENSE file. Its README states:" > third_party_licenses/github.com/dgryski/dgoogauth/NOTES.md
	@echo "" >> third_party_licenses/github.com/dgryski/dgoogauth/NOTES.md
	@echo "> Copyright (c) 2012 Damian Gryski <damian@gryski.com>" >> third_party_licenses/github.com/dgryski/dgoogauth/NOTES.md
	@echo "> This code is licensed under the Apache License, version 2.0" >> third_party_licenses/github.com/dgryski/dgoogauth/NOTES.md

# -------------------------------
# PDF documentation
# -------------------------------

PANDOC_LATEX_IMAGE=pandoc/latex:3.11

.PHONY: pdf-docs pdf-guide pdf-release-notes

pdf-docs: pdf-guide pdf-release-notes

pdf-guide:
	@echo "=> generating PDF guide"
	PANDOC_LATEX_IMAGE=$(PANDOC_LATEX_IMAGE) \
	SOURCE_DIR="`pwd`/docs" \
	INPUT_FILE="entguard-guide.md" \
	OUTPUT_FILE="entguard-v$(RELEASE_VERSION)-guide.pdf" \
	TABLE_OF_CONTENTS="true" \
	./scripts/generate-pdf.sh

pdf-release-notes:
	@echo "=> generating PDF release notes"
	PANDOC_LATEX_IMAGE=$(PANDOC_LATEX_IMAGE) \
	SOURCE_DIR="`pwd`/docs" \
	INPUT_FILE="release-notes.md" \
	OUTPUT_FILE="release-notes.pdf" \
	TABLE_OF_CONTENTS="false" \
	./scripts/generate-pdf.sh

# -------------------------------
# Database migrations
# -------------------------------
.PHONY:migrate-new
migrate-new:
	sql-migrate new -config=db/dbconfig.yaml -env="development" $(NAME)

.PHONY:migrate-up
migrate-up:
	./scripts/migrate-up.sh

.PHONY:migrate-down
migrate-down:
	./scripts/migrate-down.sh

# -------------------------------
# Database queries generation with sqlc
# -------------------------------
SQLC_VERSION=1.29.0

.PHONY:sqlc-gen
sqlc-gen:
	docker run --rm \
		-v $(PWD):/src \
		-w /src \
		sqlc/sqlc:$(SQLC_VERSION) generate

# -------------------------------
# Releases
# -------------------------------

# These relase related targets should be used only by CI/CD (with the exception
# of `release-package` if you want to test it locally).

# CI/CD is configured to run `verify` job before proceeding with the release, so
# we do not need to run tests as prerequisites for the release targets.

.PHONY: check-git-tag
check-git-tag:
	@git tag --points-at HEAD | grep -qF "v$(RELEASE_VERSION)" \
		|| (echo "git tag of current HEAD commit differs from v$(RELEASE_VERSION)" && exit 1)

# Release package does not contain docker images because they are uploaded separately to ghcr
.PHONY: release-package
release-package: egvpn pdf-docs
	RELEASE_VERSION=$(RELEASE_VERSION) \
	RELEASE_FILENAME=$(RELEASE_FILENAME) \
	./scripts/release.sh

.PHONY: release-ghcr-image-push
release-ghcr-image-push: ORCHESTRATOR_IMAGE_GHCR=$(GHCR_IMAGE_REPO_URL)/$(ORCHESTRATOR_IMAGE_PROD):$(ORCHESTRATOR_VERSION)
release-ghcr-image-push: EGSERVER_IMAGE_GHCR=$(GHCR_IMAGE_REPO_URL)/$(EGSERVER_IMAGE_PROD):$(EGSERVER_VERSION)
release-ghcr-image-push: HEALTHCHECK_IMAGE_GHCR=$(GHCR_IMAGE_REPO_URL)/$(HEALTHCHECK_IMAGE_PROD):$(HEALTHCHECK_VERSION)
release-ghcr-image-push: DBMIGRATE_IMAGE_GHCR=$(GHCR_IMAGE_REPO_URL)/$(DBMIGRATE_IMAGE):$(DBMIGRATE_VERSION)
release-ghcr-image-push: release-ghcr-image-tag
	@! docker manifest inspect $(ORCHESTRATOR_IMAGE_GHCR) \
		|| (echo "image $(ORCHESTRATOR_IMAGE_GHCR) already exists in ghcr repository" && exit 1)
	@! docker manifest inspect $(EGSERVER_IMAGE_GHCR) \
		|| (echo "image $(EGSERVER_IMAGE_GHCR) already exists in ghcr repository" && exit 1)
	@! docker manifest inspect $(HEALTHCHECK_IMAGE_GHCR) \
		|| (echo "image $(HEALTHCHECK_IMAGE_GHCR) already exists in ghcr repository" && exit 1)
	@! docker manifest inspect $(DBMIGRATE_IMAGE_GHCR) \
		|| (echo "image $(DBMIGRATE_IMAGE_GHCR) already exists in ghcr repository" && exit 1)
	docker push $(ORCHESTRATOR_IMAGE_GHCR)
	docker push $(EGSERVER_IMAGE_GHCR)
	docker push $(HEALTHCHECK_IMAGE_GHCR)
	docker push $(DBMIGRATE_IMAGE_GHCR)
