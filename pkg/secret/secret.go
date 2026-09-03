/*
 * Copyright 2020 PANTHEON.tech s.r.o.
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

package secret

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/entguard/entguard/pkg/models"
)

var (
	errCertificateUndefined = errors.New("certificate in the secrets file is undefined")
	errKeyUndefined         = errors.New("key in the secrets file is undefined")
)

// NewSecretFromFile reads sensitive data from the given path.
func NewSecretFromFile(path string) (*models.Secrets, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var s models.Secrets
	err = json.Unmarshal(data, &s)
	if err != nil {
		return nil, err
	}

	if s.Cert == "" {
		return nil, errCertificateUndefined
	}

	if s.Key == "" {
		return nil, errKeyUndefined
	}

	return &s, nil
}
