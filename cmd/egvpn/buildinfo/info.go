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

package buildinfo

import "fmt"

// All variables in this package must be set via -ldflags "-X".

// General info.
var (
	version = "unknown"
)

// Git build info.
var (
	gitCommit = "unknown"
	gitBranch = "unknown"
)

// Info about EntGuard Images.
var (
	egServerImageName = "ghcr.io/entguard/eg-server"
	egServerImageTag  = "latest"

	egHealthcheckImageName = "ghcr.io/entguard/eg-healthcheck"
	egHealthcheckImageTag  = "latest"

	egTelegrafImageName = "telegraf"
	egTelegrafImageTag  = "1.38.2-alpine"

	egVPPImageName = "ligato/vpp-base"
	egVPPImageTag  = "24.06"
)

// Version returns version of EGVPN.
func Version() string {
	return version
}

// GitCommit returns git commit hash.
func GitCommit() string {
	return gitCommit
}

// GitBranch returns git branch name.
func GitBranch() string {
	return gitBranch
}

// EgServerImageName returns full name (name:tag) of EntGuard docker image.
func EgServerImageName() string {
	return fmt.Sprintf("%s:%s", egServerImageName, egServerImageTag)
}

// EgTelegrafImageName returns full name (name:tag) of Telegraf docker image.
func EgTelegrafImageName(version string) string {
	if version == "" {
		return fmt.Sprintf("%s:%s", egTelegrafImageName, egTelegrafImageTag)
	}
	return fmt.Sprintf("%s:%s", egTelegrafImageName, version)
}

// EgHealthcheckImageName return full name (name:tag) of Healthcheck docker image.
func EgHealthcheckImageName() string {
	return fmt.Sprintf("%s:%s", egHealthcheckImageName, egHealthcheckImageTag)
}

// EgVPPImageName returns full name (name:tag) of VPP docker image.
func EgVPPImageName() string {
	return fmt.Sprintf("%s:%s", egVPPImageName, egVPPImageTag)
}
