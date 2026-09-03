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

func NewStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Provides info about WireGuard state",
		Long: `Provides information about WireGuard configuration and device information,
			provides information about container state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	isVPPUsed := isVPPUsageDetected()

	// run WG status
	if isVPPUsed {
		if err := runVPPWireguardStatus(); err != nil {
			return err
		}
	} else {
		if err := runLinuxWireguardStatus(); err != nil {
			return err
		}
	}

	// run container statuses
	runContainerStatus(serverContainerRegex, "EntGuard server")
	runContainerStatus(healthcheckContainerName, "Healthcheck")
	runContainerStatus(telegrafContainerName, "Telegraf")
	if isVPPUsed {
		runContainerStatus(vppContainerName, "VPP")
	}

	return nil
}

func runLinuxWireguardStatus() error {
	info, err := exec.Command("sudo", "wg", "show").Output()
	if err != nil {
		return fmt.Errorf("failed to get WireGuard state, maybe it is not installed: %v", err)
	}
	if s := string(info); s == "" {
		fmt.Println("WireGuard configuration and device information is empty.")
	} else {
		fmt.Println(s)
	}
	return nil
}

func runVPPWireguardStatus() error {
	var lastErr error
	info, err := exec.Command(
		"docker", "exec", vppContainerName, "vppctl", "show wireguard interface").Output()
	if err != nil {
		lastErr = fmt.Errorf("failed to retrieve VPP's wireguard interface information: %w", err)
		fmt.Println(lastErr.Error())
	} else {
		fmt.Println(string(info))
	}
	info, err = exec.Command(
		"docker", "exec", vppContainerName, "vppctl", "show wireguard peer").Output()
	if err != nil {
		lastErr = fmt.Errorf("failed to retrieve VPP's wireguard peers information: %w", err)
		fmt.Println(lastErr.Error())
	} else {
		fmt.Println(string(info))
	}
	info, err = exec.Command(
		"docker", "exec", vppContainerName, "vppctl", "show interface").Output()
	if err != nil {
		lastErr = fmt.Errorf("failed to retrieve VPP's interfaces information(traffic RX/TX info): %w", err)
		fmt.Println(lastErr.Error())
	} else {
		fmt.Println(string(info))
	}
	return lastErr
}

func runContainerStatus(containerName, containerDescription string) {
	info, err := exec.Command("bash", "-c", "docker container ls | grep "+containerName).Output()
	if err == nil {
		fmt.Print(containerDescription + " container is running: " + string(info))
		return
	}
	fmt.Println(containerDescription + " container is not running")

	info, err = exec.Command("bash", "-c", "docker container ls -a | grep "+containerName).Output()
	if err != nil {
		fmt.Println(containerDescription + " container does not exist")
	} else {
		fmt.Print(containerDescription + " container exists: " + string(info))
	}
}

func isVPPUsageDetected() bool {
	_, err := exec.Command("bash", "-c", "docker container ls | grep "+vppContainerName).Output()
	return err == nil
}
