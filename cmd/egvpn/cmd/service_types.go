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

package cmd

import (
	"fmt"
	"strings"
)

type ServiceType string

const (
	VPNServerServiceType   ServiceType = "server"
	HealthCheckServiceType ServiceType = "healthcheck"
	AllServiceType         ServiceType = "all"
)

func (m *ServiceType) String() string {
	if m == nil {
		return ""
	}
	return string(*m)
}

func (m *ServiceType) Set(value string) error {
	switch strings.ToLower(value) {
	case string(VPNServerServiceType), string(HealthCheckServiceType), string(AllServiceType):
		*m = ServiceType(value)
		return nil
	default:
		return fmt.Errorf("invalid service type")
	}
}

func (m *ServiceType) Type() string {
	return "ServiceType"
}
