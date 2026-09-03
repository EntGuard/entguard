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
	"github.com/spf13/cobra"
)

const (
	serverContainerName      = "eg-s"
	serverContainerRegex     = "eg-s$"
	healthcheckContainerName = "eg-healthcheck"
	telegrafContainerName    = "eg-s-telegraf"
	vppContainerName         = "eg-s-vpp"
	dbMigrateContainerName   = "eg-db-migrate"
)

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "egvpn",
		Short: "egvpn is an utility that manages EntGuard server",
	}

	rootCmd.AddCommand(
		NewInstallCommand(),
		NewConfigCommand(),
		NewStartCommand(),
		NewStatusCommand(),
		NewStopCommand(),
		NewUninstallCommand(),
		NewVersionCommand(),
		NewRestartCommand(),
		NewReportCommand(),
		NewCryptCommand(),
		NewCompletionCommand(rootCmd.Root().Name()),
	)

	return rootCmd
}
