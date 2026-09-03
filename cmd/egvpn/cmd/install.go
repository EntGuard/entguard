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
)

var (
	errNotInstalled = errors.New("failed to install image from archive")
)

type InstallOptions struct {
	archive string
}

func NewInstallCommand() *cobra.Command {
	var opts InstallOptions
	cmd := &cobra.Command{
		Use:   "install <archive>",
		Short: "Installs an image from given .tar archive",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.archive = args[0]
			return runInstall(opts)
		},
	}
	return cmd
}

func runInstall(opts InstallOptions) error {
	fmt.Println("Started image installation...")
	if err := exec.Command("bash", "-c", "docker load < "+opts.archive).Run(); err != nil {
		return errNotInstalled
	}
	fmt.Println("Successfully loaded image archive")
	return nil
}
