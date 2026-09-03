/*
 * Copyright 2026 PANTHEON.tech s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package test

// Sample X.509 certificates in PEM format used for testing
//
// The certificates can be inspected using one of these commands:
//
//    $ echo CERTIFICATE | openssl x509 -text -noout
//    $ echo CRL | openssl crl -text -noout
//
// Example:
//
//    $ echo "-----BEGIN CERTIFICATE-----
//    MIIDTDCCAv6gAwIBAgICEAAwBQYDK2VwMIGEMQswCQYDVQQGEwJVUzELMAkGA1UE
//    [...]
//    YxGFBK+1sxml3oZY7jjhD+oxlmZx0l9fd9cvkLJgjg8=
//    -----END CERTIFICATE-----" | openssl x509 -text -noout

// Commands how to generate new certificates:
//  Some of these commands will prompt you for information (organization name,
// 	state, email address). Enter some mock testing data, not real information as
// 	these certs will just be used for testing.

// 	# generate ed25519 root CA key
// 	openssl genpkey -algorithm ed25519 -out rootCA.key

// 	# create new root CA
// 	openssl req -x509 -new -key rootCA.key -out rootCA.crt -days 3651

// 	# generate ed25519 cert keys - one for cert that will be revoked, other for valid cert
// 	openssl genpkey -algorithm ed25519 -out revoked.key
// 	openssl genpkey -algorithm ed25519 -out valid.key

// 	# generate certificate signing requests
// 	openssl req -new -key revoked.key -out revoked.csr
// 	openssl req -new -key valid.key -out valid.csr

// 	# sign the certificates
// 	openssl x509 -req -in revoked.csr -CA rootCA.crt -CAkey rootCA.key -CAcreateserial -out revoked.crt -days 3650
// 	openssl x509 -req -in valid.csr -CA rootCA.crt -CAkey rootCA.key -CAcreateserial -out valid.crt -days 3650

// 	# create file openssl.cnf with these contents:
// 	[ ca ]
// 	default_ca = CA_default

// 	[ CA_default ]
// 	dir               = .
// 	database          = $dir/index.txt
// 	new_certs_dir     = $dir/newcerts
// 	serial            = $dir/serial
// 	crlnumber         = $dir/crlnum
// 	default_crl_days  = 3650             # 10 years validity for test purposes
// 	default_md        = default
// 	preserve          = no
// 	policy            = policy_anything

// 	[ policy_anything ]
// 	countryName            = optional
// 	stateOrProvinceName    = optional
// 	organizationName       = optional
// 	organizationalUnitName = optional
// 	commonName             = supplied
// 	emailAddress           = optional

// 	# create these files (they are referred to in the openssl.cnf file)
// 	touch index.txt
// 	mkdir -p newcerts
// 	echo 01 > serial
// 	echo 01 > crlnum

// 	# generate empty CRL (not sure if this step is required, seems to work without it as well)
// 	openssl ca -gencrl -out rootCA.crl -keyfile rootCA.key -cert rootCA.crt -config openssl.cnf

// 	# revoke certificate
// 	openssl ca -revoke revoked.crt -keyfile rootCA.key -cert rootCA.crt -config openssl.cnf

// 	# update CRL
// 	openssl ca -gencrl -out rootCA.crl -keyfile rootCA.key -cert rootCA.crt -config openssl.cnf

// 	# check CRL
// 	openssl crl -in rootCA.crl -text -noout

// 	# check that one cert is revoked and other is valid
// 	openssl verify -crl_check -CAfile rootCA.crt -CRLfile rootCA.crl revoked.crt
// 	openssl verify -crl_check -CAfile rootCA.crt -CRLfile rootCA.crl valid.crt

const CA = `-----BEGIN CERTIFICATE-----
MIICHzCCAdGgAwIBAgIUPTAkrs7X3SWNY+jKHGz/8Pxz54gwBQYDK2VwMIGEMQsw
CQYDVQQGEwJVUzELMAkGA1UECAwCT1IxGjAYBgNVBAoMEVRlc3QgT3JnYW5pemF0
aW9uMRIwEAYDVQQLDAlUZXN0IFVuaXQxFTATBgNVBAMMDFRlc3QgUm9vdCBDQTEh
MB8GCSqGSIb3DQEJARYSdGVzdEBleGFtcGxlLmxvY2FsMB4XDTI1MDQyOTA4Mjg0
MloXDTM1MDQyODA4Mjg0MlowgYQxCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJPUjEa
MBgGA1UECgwRVGVzdCBPcmdhbml6YXRpb24xEjAQBgNVBAsMCVRlc3QgVW5pdDEV
MBMGA1UEAwwMVGVzdCBSb290IENBMSEwHwYJKoZIhvcNAQkBFhJ0ZXN0QGV4YW1w
bGUubG9jYWwwKjAFBgMrZXADIQDWGIi3eCqr3Z6vYWsMqb0YyYgojYZqjWf/phDl
Y5yeZ6NTMFEwHQYDVR0OBBYEFK+bQd+RSdFD7c+UwR33rk59PfU8MB8GA1UdIwQY
MBaAFK+bQd+RSdFD7c+UwR33rk59PfU8MA8GA1UdEwEB/wQFMAMBAf8wBQYDK2Vw
A0EAheatiPTyJR7NH/ytfkU5Qr7f7+ezDlbxRANNzOJ6Ym3CBhZfcW+12n3Ad/eD
o9aYK5SlQz9BgEv825TXdkGbDw==
-----END CERTIFICATE-----
`

const CRL = `-----BEGIN X509 CRL-----
MIIBNTCB6AIBATAFBgMrZXAwgYQxCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJPUjEa
MBgGA1UECgwRVGVzdCBPcmdhbml6YXRpb24xEjAQBgNVBAsMCVRlc3QgVW5pdDEV
MBMGA1UEAwwMVGVzdCBSb290IENBMSEwHwYJKoZIhvcNAQkBFhJ0ZXN0QGV4YW1w
bGUubG9jYWwXDTI1MDQyOTExMjU1NloXDTM1MDQyNzExMjU1NlowJzAlAhQu8/Fs
02SfrddhgB/4SrH/JR4x2RcNMjUwNDI5MTEyNTEwWqAOMAwwCgYDVR0UBAMCAQIw
BQYDK2VwA0EAH41nMPFtYc8pp/YoshRihbOt3Pv897eEB8kBfL3mRetiOJrJdLS+
0fhrTCVWQ914qfTFszARPjuJVl8e0Es5Cw==
-----END X509 CRL-----
`

const ValidCert = `-----BEGIN CERTIFICATE-----
MIICCjCCAbygAwIBAgIULvPxbNNkn63XYYAf+Eqx/yUeMdowBQYDK2VwMIGEMQsw
CQYDVQQGEwJVUzELMAkGA1UECAwCT1IxGjAYBgNVBAoMEVRlc3QgT3JnYW5pemF0
aW9uMRIwEAYDVQQLDAlUZXN0IFVuaXQxFTATBgNVBAMMDFRlc3QgUm9vdCBDQTEh
MB8GCSqGSIb3DQEJARYSdGVzdEBleGFtcGxlLmxvY2FsMB4XDTI1MDQyOTA4NDMy
OVoXDTM1MDQyNzA4NDMyOVowgYAxCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJPUjEa
MBgGA1UECgwRVGVzdCBPcmdhbml6YXRpb24xEjAQBgNVBAsMCVRlc3QgVW5pdDER
MA8GA1UEAwwIVGVzdCBDU1IxITAfBgkqhkiG9w0BCQEWEnRlc3RAZXhhbXBsZS5s
b2NhbDAqMAUGAytlcAMhAPb8PfVDEwgYUM6iAiZYaQI+9ikH2aOdl5NSfL9PQO/N
o0IwQDAdBgNVHQ4EFgQUQPKy7WTkc+31tez1zOzABK4XXYgwHwYDVR0jBBgwFoAU
r5tB35FJ0UPtz5TBHfeuTn099TwwBQYDK2VwA0EA/KHK9AyZM3fh1p8oJ0avfVOL
BQXPcfYnxoDuJs8FCGNHS9Pa059rW+Jst8LhGaCwe3N6onGHnSyRQnSgLyA5AQ==
-----END CERTIFICATE-----
`

const RevokedCert = `-----BEGIN CERTIFICATE-----
MIICCjCCAbygAwIBAgIULvPxbNNkn63XYYAf+Eqx/yUeMdkwBQYDK2VwMIGEMQsw
CQYDVQQGEwJVUzELMAkGA1UECAwCT1IxGjAYBgNVBAoMEVRlc3QgT3JnYW5pemF0
aW9uMRIwEAYDVQQLDAlUZXN0IFVuaXQxFTATBgNVBAMMDFRlc3QgUm9vdCBDQTEh
MB8GCSqGSIb3DQEJARYSdGVzdEBleGFtcGxlLmxvY2FsMB4XDTI1MDQyOTA4NDMx
MVoXDTM1MDQyNzA4NDMxMVowgYAxCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJPUjEa
MBgGA1UECgwRVGVzdCBPcmdhbml6YXRpb24xEjAQBgNVBAsMCVRlc3QgVW5pdDER
MA8GA1UEAwwIVGVzdCBDU1IxITAfBgkqhkiG9w0BCQEWEnRlc3RAZXhhbXBsZS5s
b2NhbDAqMAUGAytlcAMhAD9k5KKpDJ5m2Uni6PxEJv8GL/RJ787SybonoSqI/B3B
o0IwQDAdBgNVHQ4EFgQUA7/Y6N48GPZQ7fKgVZT7GDErfScwHwYDVR0jBBgwFoAU
r5tB35FJ0UPtz5TBHfeuTn099TwwBQYDK2VwA0EAYnAOZ2xoJX2uhS0Kh++6GVvS
fyaewoCnDxbHX2+3Hw+xfibAvw7i/iiUJ7bafcgX5Mx2is+jfhdMnjuVsH1jBA==
-----END CERTIFICATE-----
`
