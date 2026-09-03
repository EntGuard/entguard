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

echo "Generating development certificates."
echo "Using mkcert (https://github.com/FiloSottile/mkcert)."

dpkg -S libnss3-tools &> /dev/null
if [ $? -ne 0 ] ; then echo "Missing Network Security Service tools:"; echo "    sudo apt install libnss3-tools"; exit -1; fi

echo "Downloading mkcert v1.4.1 binary."

CERT_DIR="${CERT_DIR:-./dev-certs}"
MKCERT_LINK="https://github.com/FiloSottile/mkcert/releases/download/v1.4.1/mkcert-v1.4.1-linux-amd64"
MKCERT="${CERT_DIR}/mkcert-v1.4.1"

curl -o ${MKCERT} --create-dirs --location --progress-bar ${MKCERT_LINK}
chmod +x ${MKCERT}

DOMAIN_NAME=${1:-"cnf-vpn-o.local localhost 127.0.0.1"}

CAROOT=${CERT_DIR} ${MKCERT} \
    -key-file ${CERT_DIR}/grpc-server-key.pem \
    -cert-file ${CERT_DIR}/grpc-server-cert.pem \
    $DOMAIN_NAME
