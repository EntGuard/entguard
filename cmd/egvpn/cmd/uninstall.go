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

package cmd

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/entguard/entguard/cmd/egvpn/buildinfo"
)

func NewUninstallCommand() *cobra.Command {
	var opts egVPNOptions

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Removes specified image",
		Long:  "Removes EntGuard server and HealthCheck images",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstall(opts)
		},
	}

	flags := cmd.Flags()

	opts.Service = AllServiceType
	flags.VarP(&opts.Service, "service", "s", fmt.Sprintf("service to delete image for. ServiceType: %s|%s|%s", VPNServerServiceType, HealthCheckServiceType, AllServiceType))

	return cmd
}

func runUninstall(opts egVPNOptions) (resultErr error) {
	errNotRemoved := "failed to remove the image %s, check if it exists and is not running"

	if opts.Service == VPNServerServiceType ||
		opts.Service == AllServiceType {
		if err := exec.Command("docker", "image", "rm", buildinfo.EgServerImageName()).Run(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf(errNotRemoved, buildinfo.EgServerImageName()))
		} else {
			fmt.Printf("Successfully removed %s image\n\n", buildinfo.EgServerImageName())
		}
	}

	if opts.Service == HealthCheckServiceType ||
		opts.Service == AllServiceType {
		if err := exec.Command("docker", "image", "rm", buildinfo.EgHealthcheckImageName()).Run(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf(errNotRemoved, buildinfo.EgHealthcheckImageName()))
		} else {
			fmt.Printf("Successfully removed %s image\n\n", buildinfo.EgHealthcheckImageName())
		}
	}

	return resultErr
}
