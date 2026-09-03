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

VPN_O_ENDPOINT="https://127.0.0.1:8080"
ADMIN_USERNAME="admin"
ADMIN_PASSWORD="5Bt3kp0ItQ;9"
CERT_FILE_PATH="${1:-INSECURE}"
if [ "$CERT_FILE_PATH" = "INSECURE" ]; then
    SSL_FLAG="--insecure"
else
    SSL_FLAG="--cacert $CERT_FILE_PATH"
fi

echo "This script shows JWT auth example."

set -e

echo "Requesting JWT for the user."
JWT=$(curl $SSL_FLAG -X POST --silent -d \
    '{
        "username": "admin",
        "password": "5Bt3kp0ItQ;9"
    }' \
    ${VPN_O_ENDPOINT}/api/v1/jwt | jq -r ".jwt")

echo "List of users:"
curl $SSL_FLAG --silent -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/users | jq