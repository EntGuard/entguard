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

TRIVY_VERSION="${TRIVY_VERSION}"
TRIVY_CACHE_VOL="trivy-cache"
TRIVY_CMD_BASE="docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v $TRIVY_CACHE_VOL:/root/.cache/trivy"

COMPOSE_FILE="provisioning/docker/compose.yaml"
EGSERVER_IMAGE="${EGSERVER_IMAGE}"
HEALTHCHECK_IMAGE="${HEALTHCHECK_IMAGE}"
GOBIN_DIR="$(go env GOPATH)/bin"

IMAGES=( $(cat $COMPOSE_FILE | grep "^ *image:" | cut -d ":" -f 2-) )

N_MANUAL_TESTS=3
i=0
n=$(( ${#IMAGES[@]} + $N_MANUAL_TESTS ))

docker volume create $TRIVY_CACHE_VOL > /dev/null
$TRIVY_CMD_BASE aquasec/trivy:${TRIVY_VERSION} image --download-db-only

for IMAGE in ${IMAGES[@]}
do
    ((i++))
    echo
    echo "==============================================================================="
    echo "Scan $i/$n: Image $IMAGE"
    echo "==============================================================================="
    echo
    $TRIVY_CMD_BASE aquasec/trivy:${TRIVY_VERSION} image $IMAGE
done

((i++))
echo
echo "==============================================================================="
echo "Scan $i/$n: Image $EGSERVER_IMAGE"
echo "==============================================================================="
echo
$TRIVY_CMD_BASE aquasec/trivy:${TRIVY_VERSION} image $EGSERVER_IMAGE

((i++))
echo
echo "==============================================================================="
echo "Scan $i/$n: Image $HEALTHCHECK_IMAGE"
echo "==============================================================================="
echo
$TRIVY_CMD_BASE aquasec/trivy:${TRIVY_VERSION} image $HEALTHCHECK_IMAGE

((i++))
echo
echo "==============================================================================="
echo "Scan $i/$n: Binary $GOBIN_DIR/egvpn"
echo "==============================================================================="
echo
$TRIVY_CMD_BASE -v $GOBIN_DIR/egvpn:/egvpn aquasec/trivy:${TRIVY_VERSION} rootfs egvpn

docker volume rm $TRIVY_CACHE_VOL > /dev/null
