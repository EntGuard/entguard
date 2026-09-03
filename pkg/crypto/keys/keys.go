/*
 * Copyright 2024 PANTHEON.tech s.r.o.
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

package keys

import (
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// GenerateBase64EncodedWGKeyPair generates cryptographic key pair (for usage in Wireguard) and encodes it
// into base64 format before returning it.
func GenerateBase64EncodedWGKeyPair() (privateKey string, publicKey string, err error) {
	privateKeyObj, genErr := wgtypes.GeneratePrivateKey()
	if genErr != nil {
		return "", "", genErr
	}
	return privateKeyObj.String(), privateKeyObj.PublicKey().String(), nil
}

// GenerateBase64EncodedWGPresharedKey generates cryptographic preshared key (for usage in Wireguard) and encodes it
// into base64 format before returning it.
func GenerateBase64EncodedWGPresharedKey() (presharedKey string, err error) {
	presharedKeyObj, genErr := wgtypes.GenerateKey()
	if genErr != nil {
		return "", genErr
	}
	return presharedKeyObj.String(), nil
}
