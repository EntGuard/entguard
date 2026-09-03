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
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

func NewStopCommand() *cobra.Command {
	var opts egVPNOptions

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stops containers if they are running and removes them",
		Long: "Stops EntGuard server, HealthCheck, Telemetry and VPP containers if they are running and removes them if they exist." +
			" Removes docker volumes for telemetry and VPP sockets.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(opts)
		},
	}

	flags := cmd.Flags()

	opts.Service = AllServiceType
	flags.VarP(&opts.Service, "service", "s", fmt.Sprintf("service to stop. ServiceType: %s|%s|%s", VPNServerServiceType, HealthCheckServiceType, AllServiceType))

	return cmd
}

func runStop(opts egVPNOptions) error {
	if opts.Service == VPNServerServiceType ||
		opts.Service == AllServiceType {
		stopContainer(serverContainerName, "EntGuard server")
		removeContainer(serverContainerName, "EntGuard server")

		stopContainer(vppContainerName, "VPP")
		removeContainer(vppContainerName, "VPP")

		stopContainer(telegrafContainerName, "Telegraf")
		removeContainer(telegrafContainerName, "Telegraf")

		removeVolume(telegrafVolumeName, "volume for telemetry socket")
		removeVolume(vppSocketDirVolumeName, "volume for VPP socket")
	}

	if opts.Service == HealthCheckServiceType ||
		opts.Service == AllServiceType {
		stopContainer(healthcheckContainerName, "EntGuard healthcheck service")
		removeContainer(healthcheckContainerName, "EntGuard healthcheck service")
	}

	return nil
}

func removeVolume(volumeName, volumeDescription string) {
	fmt.Printf("Trying to remove the docker %s\n", volumeDescription)
	if err := exec.Command("docker", "volume", "rm", volumeName).Run(); err != nil {
		fmt.Printf("Failed to remove the docker %s: %v \n", volumeDescription, err)
	} else {
		fmt.Println("Done.")
	}
}

func stopContainer(containerName, containerDesciption string) {
	fmt.Printf("Trying to stop the %s container\n", containerDesciption)
	if err := exec.Command("docker", "stop", containerName).Run(); err != nil {
		fmt.Printf("Failed to stop the container: %v \n", err)
	} else {
		fmt.Println("Done.")
	}
}

func removeContainer(containerName, containerDesciption string) {
	fmt.Printf("Trying to remove the %s container\n", containerDesciption)
	if err := exec.Command("docker", "rm", containerName).Run(); err != nil {
		fmt.Printf("Failed to remove the container: %v \n", err)
	} else {
		fmt.Println("Done.")
	}
}
