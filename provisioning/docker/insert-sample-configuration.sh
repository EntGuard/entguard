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

# This script must be applied to a clean instance. Do not configure anything before running this script.
# By default it uses insecure https (no certificate checking), but you can provide orchestrator API certificate for
# proper security.

# This file serves these purposes:
# - to quickly insert data for testing purposes
# - to show how to use the REST API

VPN_O_ENDPOINT="https://127.0.0.1:8080"
ADMIN_USERNAME="admin"
ADMIN_PASSWORD="5Bt3kp0ItQ;9"
CERT_FILE_PATH="${1:-INSECURE}"
if [ "$CERT_FILE_PATH" = "INSECURE" ]; then
    SSL_FLAG="--insecure"
else
    SSL_FLAG="--cacert $CERT_FILE_PATH"
fi

set -e

# Get JWT for admin user; this will be used to authenticate next requests
JWT=$(curl --silent $SSL_FLAG -X POST -d \
    '{
        "username": "'${ADMIN_USERNAME}'",
        "password": "'${ADMIN_PASSWORD}'"
    }' \
    ${VPN_O_ENDPOINT}/api/v1/jwt | jq -r ".jwt")

# Create a server
curl --silent $SSL_FLAG -X POST -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/servers -d \
'{
    "name": "test_server",
    "endpoint": "127.0.0.1",
    "healthcheck_address": "10.20.30.40",
    "description": "This is a sample server",
    "vpn_config": {
        "name":"egs0",
        "private_key":"0FsoFCq1EZb30fKvs8I43iCAof7v1pukGPsSEOAoH1g=",
        "public_key":"2SaLuA4nGVf77azT9kmW28V9DW/0aSbMH43GTt7rA3w=",
        "addresses":["1.1.1.1/24"],
        "listen_port":"9090",
        "dns":["dns1"],
        "mtu":"1350",
        "persistent_keepalive":"20"
    }
}'
SERVER_ID=$(curl --silent $SSL_FLAG -X GET -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/servers | jq -r .servers[0].id)

# Create an address pool
curl --silent $SSL_FLAG -X POST -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/address-pools -d \
'{
    "name": "pool1",
    "description": "a sample address pool",
    "start_addr": "1.1.1.101",
    "end_addr": "1.1.1.255",
    "net_mask": 24
}'
POOL_ID=$(curl --silent $SSL_FLAG -X GET -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/address-pools | jq -r .address_pools[0].id)

# Create a user # space in username should not cause issues
curl --silent $SSL_FLAG -X POST -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/users -d \
'{
    "username":"user local",
    "password":"password123456789",
    "is_admin": false,
    "mfa_type": "",
    "notification": "ntf"
}'
USER_ID=$(curl --silent $SSL_FLAG -X GET -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/users | jq -r .users[1].id)

# Create a device template for the user
curl --silent $SSL_FLAG -X POST -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/users/${USER_ID}/device-template -d \
'{
    "interface_name":"eg0",
    "address_pools":[
        {
            "id":'${POOL_ID}'
        }
    ],
    "listen_port":"9090",
    "dns":["dns1"],
    "mtu":"1350"
}'
DEVICE_TEMPLATE_ID=$(curl --silent $SSL_FLAG -X GET -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/users/${USER_ID}/device-template | jq -r .id)

# Create two new devices
DEVICE1=$(curl --silent $SSL_FLAG -X GET -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/device-templates/${DEVICE_TEMPLATE_ID}/devices/autogen)
curl --silent $SSL_FLAG -X POST -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/device-templates/${DEVICE_TEMPLATE_ID}/devices -d ${DEVICE1}
DEVICE_ID=$(curl --silent $SSL_FLAG -X GET -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/device-templates/${DEVICE_TEMPLATE_ID}/devices | jq -r .devices[0].id)

# Update the first device (usually not needed, the auto-generated attributes should be good)
curl --silent $SSL_FLAG -X PATCH -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/devices/${DEVICE_ID} -d \
'{
    "description":"new desc",
    "private_key":"4ElAvYDPPAIeae2ZExD4IUpUFNsc0uV6n/cCYlMf/2s=",
    "public_key":"YjXCP5IOY0B4H/OVLdMVtNH+B6esgA6sOn0GRZND/Vs=",
    "addresses":["1.1.1.150/24", "5.5.5.5/16"]
}'

# Resync devices for the device template (this overwrites previously set addresses if they are not within template's address ranges)
curl --silent $SSL_FLAG -X POST -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/device-templates/${DEVICE_TEMPLATE_ID}/devices/resync

# Create an adjacency template
curl --silent $SSL_FLAG -X POST -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/vpn/adjacency-templates -d \
'{
    "server_id":'${SERVER_ID}',
    "user_id":'${USER_ID}',
    "template_config": {
        "use_preshared_key":true,
        "client_side_allowed_ips":["1.2.3.0/24", "10.10.0.0/16", "10.20.30.40"]
    }
}'

# Create an actual (device) adjacency (usually not needed, doing adjacencies resync should handle it)
curl --silent $SSL_FLAG -X POST -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/vpn/adjacencies -d \
'{
    "server_id":'${SERVER_ID}',
    "device_id":'${DEVICE_ID}',
    "config": {
        "preshared_key":"aRkLot3NQUKXIxdNvrhsK1kkDyzVoR+t6ciswbPFUCk=",
        "allowed_ips":["99.0.0.0/8", "10.20.30.40"],
        "server_side":false
    }
}'

# Resync device adjacencies for the user (this overwrites previously created adjacency)
curl --silent $SSL_FLAG -X POST -H "Authorization: Bearer ${JWT}" ${VPN_O_ENDPOINT}/api/v1/users/${USER_ID}/device-adjacencies/resync
