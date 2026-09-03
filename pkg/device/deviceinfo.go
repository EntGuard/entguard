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

package device

import (
	"fmt"
	"strings"
)

// ParseInformation parses raw device information string into formatted multiline output
// Input format: "OS:iOS;OSV:17.2;OSB:21C66;APPV:3.5.1;DVCE:iPhone 15 Pro"
// Output format:
// OS:          iOS
// OS Version:  17.2
// OS Build:    21C66
// APP Version: 3.5.1
// Device:      iPhone 15 Pro
func ParseInformation(raw string) string {
	if raw == "" {
		return ""
	}

	pairs := make(map[string]string)
	parts := strings.Split(raw, ";")
	for _, part := range parts {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) == 2 {
			pairs[kv[0]] = kv[1]
		}
	}

	var result strings.Builder
	if os, ok := pairs["OS"]; ok {
		fmt.Fprintf(&result, "OS:          %s\n", os)
	}
	if osv, ok := pairs["OSV"]; ok {
		fmt.Fprintf(&result, "OS Version:  %s\n", osv)
	}
	if osb, ok := pairs["OSB"]; ok {
		fmt.Fprintf(&result, "OS Build:    %s\n", osb)
	}
	if appv, ok := pairs["APPV"]; ok {
		fmt.Fprintf(&result, "APP Version: %s\n", appv)
	}
	if dvce, ok := pairs["DVCE"]; ok {
		fmt.Fprintf(&result, "Device:      %s\n", dvce)
	}

	output := result.String()
	if len(output) > 0 {
		output = output[:len(output)-1]
	}
	return output
}
