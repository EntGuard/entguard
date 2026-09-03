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

	"github.com/spf13/cobra"
)

const (
	// dontOverrideStartHugepagesFlag is special value for vpp-hugepages-2m-count restart flag that is saying
	// that this flag should not override equally-named start flag (hugepages values of this flag that are below 0
	// are not valid hugepages count -> they are used for special-meaning values)
	dontOverrideStartHugepagesFlag = -2
)

func NewRestartCommand() *cobra.Command {
	var opts egVPNOptions

	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Removes the EntGuard server and Healthcheck containers and starts them again",
		Long:  "Removes the EntGuard server and Healthcheck containers and starts them again with options that were set during start, unless they are overwritten",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestart(opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.SecretsPath, "tls-auth-path", "t", "", "path to the tls-auth file")
	flags.StringVarP(&opts.ConfigPath, "config-path", "c", "", "path to the config file")
	flags.StringVarP(&opts.VPPConfigPath, "vpp-config-path", "p", "", "path to the VPP config file")
	flags.StringVarP(&opts.VPPDayZeroConfigPath, "vpp-day0-config-path", "z", "", "path to the VPP day0 config file")

	flags.StringVarP(&opts.TelegrafConfigPath, "tel-config", "m", "", "path to the Telegraf config file")
	flags.StringVarP(&opts.TelegrafServerName, "server-name", "n", "", "server name that will be shown in Grafana. It will overwrite the name in the egvpn config")
	flags.StringVarP(&opts.TelegrafVersion, "tel-version", "v", "", "Telegraf image version, if not set will be used the default one")

	flags.IntVarP(&opts.VPPConnectionCheckTimeout, "vpp-connection-check-timeout", "o", -1, "Timeout (in seconds) for checking connection to VPP using VPP socket. Connection is also used for applying day0 config to VPP.(Default -1 means that we don't override start flag with this flag)")

	flags.IntVarP(&opts.HugePages2MCount, "vpp-hugepages-2m-count", "u", dontOverrideStartHugepagesFlag, fmt.Sprintf("Count of 2MiB hugepages that should be checked for and allocated for VPP usage. (default is %d, using %d will disable check and dynamic allocation)", defaultVPPHugepages2MCount, dontCheckAndSetVPPHugepages))

	opts.Service = AllServiceType
	flags.VarP(&opts.Service, "service", "s", fmt.Sprintf("service to restart. ServiceType: %s|%s|%s", VPNServerServiceType, HealthCheckServiceType, AllServiceType))

	return cmd
}

func runRestart(opts egVPNOptions) error {
	fmt.Print("Loading restart options from file... ")
	savedOpts, err := loadRestartOpts()
	if err != nil {
		return err
	}
	fmt.Println("Loaded.")

	resOpts := overwriteOptions(savedOpts, &opts)

	fmt.Printf("Restarting the EntGuard services containers with following options:\n"+
		"\n"+
		"\tService: %q\n"+
		"\tSecrets path: %q\n"+
		"\tConfig path: %q\n"+
		"\tVPP config path: %q\n"+
		"\tVPP day0 config path: %q\n"+
		"\tEnable telemetry: %v\n"+
		"\tTelegraf config path: %q\n"+
		"\tTelegraf server name: %q\n"+
		"\tTelegraf version: %q\n"+
		"\tVPP connection check timeout: %d seconds\n"+
		"\tHugePages 2M count: %d\n"+
		"\n",
		resOpts.Service,
		resOpts.SecretsPath,
		resOpts.ConfigPath,
		resOpts.VPPConfigPath,
		resOpts.VPPDayZeroConfigPath,
		resOpts.EnableTelemetry,
		resOpts.TelegrafConfigPath,
		resOpts.TelegrafServerName,
		resOpts.TelegrafVersion,
		resOpts.VPPConnectionCheckTimeout,
		resOpts.HugePages2MCount)

	if err := runStop(*resOpts); err != nil {
		return err
	}

	if err := runStart(*resOpts); err != nil {
		return err
	}

	return nil
}

func overwriteOptions(oldOpts, owOpts *egVPNOptions) *egVPNOptions {
	if owOpts.ConfigPath != "" {
		oldOpts.ConfigPath = owOpts.ConfigPath
	}
	if owOpts.SecretsPath != "" {
		oldOpts.SecretsPath = owOpts.SecretsPath
	}
	if owOpts.VPPConfigPath != "" {
		oldOpts.VPPConfigPath = owOpts.VPPConfigPath
	}
	if owOpts.VPPDayZeroConfigPath != "" {
		oldOpts.VPPDayZeroConfigPath = owOpts.VPPDayZeroConfigPath
	}
	if owOpts.TelegrafConfigPath != "" {
		oldOpts.TelegrafConfigPath = owOpts.TelegrafConfigPath
	}
	if owOpts.TelegrafServerName != "" {
		oldOpts.TelegrafServerName = owOpts.TelegrafServerName
	}
	if owOpts.TelegrafVersion != "" {
		oldOpts.TelegrafVersion = owOpts.TelegrafVersion
	}
	if owOpts.VPPConnectionCheckTimeout != -1 {
		oldOpts.VPPConnectionCheckTimeout = owOpts.VPPConnectionCheckTimeout
	}
	if owOpts.HugePages2MCount != dontOverrideStartHugepagesFlag {
		oldOpts.HugePages2MCount = owOpts.HugePages2MCount
	}
	if owOpts.Service != "" {
		oldOpts.Service = owOpts.Service
	}

	return oldOpts
}
