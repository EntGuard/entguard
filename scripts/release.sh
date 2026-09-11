#!/bin/bash

# Copyright 2026 PANTHEON.tech s.r.o.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

RELEASE_VERSION="${RELEASE_VERSION}"
RELEASE_DIR="./${RELEASE_FILENAME}"
GOBIN_DIR="$(go env GOPATH)/bin"
PDF_GUIDE="./docs/entguard-v${RELEASE_VERSION}-guide.pdf"
PDF_RELEASE_NOTES="./docs/release-notes.pdf"

if [ -d "${RELEASE_DIR}" ]; then
    echo "ERROR: Release directory already exists (${RELEASE_DIR}). Please remove it first."
    exit 1
fi

if [ ! -f "${PDF_GUIDE}" ]; then
    echo "ERROR: EntGuard installation guide not found (${PDF_GUIDE}). To build it, use:"
    echo "    make pdf-guide"
    exit 1
fi

if [ ! -f "${PDF_RELEASE_NOTES}" ]; then
    echo "ERROR: EntGuard release notes not found (${PDF_RELEASE_NOTES}). To build them, use:"
    echo "    make pdf-release-notes"
    exit 1
fi

echo " => Creating release directory ${RELEASE_DIR}"
mkdir "${RELEASE_DIR}"

echo " => Copying egvpn binary"
cp "${GOBIN_DIR}/egvpn" "${RELEASE_DIR}/egvpn"

echo " => Copying DB migration config file"
cp ./provisioning/docker/dbconfig.yaml "${RELEASE_DIR}/dbconfig.yaml"

echo " => Copying Compose file"
cp ./provisioning/docker/compose.yaml "${RELEASE_DIR}/compose.yaml"

echo " => Copying script for making development certificates"
cp ./scripts/generate-dev-certs.sh "${RELEASE_DIR}/generate-dev-certs.sh"

echo " => Copying installation guide"
cp "${PDF_GUIDE}" "${RELEASE_DIR}"

echo " => Copying release notes"
cp "${PDF_RELEASE_NOTES}" "${RELEASE_DIR}"

echo " => Copying license file"
cp "LICENSE" "${RELEASE_DIR}"

echo " => Copying notice file"
cp "NOTICE" "${RELEASE_DIR}"

echo " => Creating empty directory for importing db dump"
mkdir ${RELEASE_DIR}/previous-db

echo " => Copying Grafana, Prometheus and Telegraf configs"
cp -r ./provisioning/docker/grafana/ "${RELEASE_DIR}/grafana"
cp -r ./provisioning/docker/telegraf/ "${RELEASE_DIR}/telegraf"
cp -r ./provisioning/docker/prometheus/ "${RELEASE_DIR}/prometheus"

echo " => Archiving release to ${RELEASE_DIR}.zip"
zip "${RELEASE_DIR}.zip" -r "${RELEASE_DIR}"
