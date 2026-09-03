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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/entguard/entguard/cmd/egvpn/client"
	"github.com/entguard/entguard/pkg/prompter"
	config "github.com/entguard/entguard/pkg/server-config"
)

const (
	// If you are changing default names of generated files,
	// be a good developer, update .gitignore file in the project root too.
	defaultTLSAuthFileName          = "tls-auth.json"
	defaultConfigFileName           = "config.json"
	defaultVPPConfigFileName        = "vpp.config"
	defaultVPPDayZeroConfigFileName = "vpp-day0.config"

	defaultAPIAddress       = "https://localhost:8080"
	defaultCacertFilePath   = "dev-certs/rootCA.pem"
	defaultOrchCertFilePath = "dev-certs/grpc-server-cert.pem"
)

// Default values for configuration.
// Copy-pasted from "pkg/server-config/config.go" file.
const (
	// Do not use "localhost" instead of "127.0.0.1". There is a bug in golang
	// grpc library with DNS resolving. If you run the egserver in a VM with
	// default (NAT) networking, network adapter is up in guest OS and network
	// connectivity is down in host OS, resolving "localhost" takes 10 seconds.
	defaultGRPCAddress = "127.0.0.1:8081"
	// Must be at least 10 seconds plus some headroom to double guard against
	// the aforementioned bug.
	defaultGRPCTimeout       = 15 * time.Second
	defaultCaName            = "cnf-vpn-o.local"
	defaultReconnectAttempts = 10
	defaultReconnectPause    = 5 * time.Second
	defaultLogLevel          = "info"

	defaultTokensPerSecond = 20
	defaultBucketSize      = 500

	defaultHCGRPCAddress = "127.0.0.1:8083"
	defaultHCListenPort  = 8084

	defaultHCTokensPerSecond = 20  // 200 devices with 20 seconds healthcheck refresh = 10 tokens per second (multiply by 2 for buffer and irregular hc ping distribution)
	defaultHCBucketSize      = 500 // burst buffer for temporal high traffic
)

const (
	egvpnConfigPath = "/etc/entguard/egvpn-config.json"
	telegrafConfig  = `
[global_tags]
  server="$VPN_SERVER_NAME"
[agent]
  interval = "5s"
  round_interval = true
  metric_batch_size = 1000
  metric_buffer_limit = 1000
  collection_jitter = "0s"
  flush_interval = "5s"
  flush_jitter = "0s"
  precision = ""
  hostname = ""
  omit_hostname = false
[[outputs.socket_writer]]
  address = "unix:///run/sockets/socket"
[[inputs.net]]
  ignore_protocol_stats = true
[[inputs.nstat]]
[[inputs.wireguard]]
[[inputs.mem]]
[[inputs.cpu]]
  percpu = true
  totalcpu = true
  collect_cpu_time = false
  report_active = false
`
	vppDayZeroConfig = `
## this is day0 configuration file that contains list of VPP CLI commands(one line=one VPP CLI command) that 
## that will be applied just after the start of the VPP (day0 configuration)

## feel free to uncomment (remove #) lines, use example VPP CLI commands or modify them as you need
## full VPP CLI reference is at https://s3-docs.fd.io/vpp/24.06/cli-reference/index.html

## setup further attributes for VPP(DPDK) interface listed in "egvpn config init" command as "vpp-interfaces" parameter
## each interface will be named inside VPP as dpdk{order number}, i.e. dpdk1,dpdk2,dpdk3,... (in order as they 
## were provided in "vpp-interfaces" parameter in "egvpn config init" command)
set interface state dpdk1 up
set interface ip address dpdk1 10.251.0.1/24
set interface state dpdk2 up
set interface ip address dpdk2 10.252.0.1/24

## mark endpoint of this node's wireguard peer
set interface tag dpdk1 wg-underlay
`
)

var (
	errEmptyServerName          = errors.New("name of the EntGuard server cannot be empty")
	errEmptyUserName            = errors.New("name of the admin user cannot be empty")
	errEmptyPassword            = errors.New("user password cannot be empty")
	errMutualExclusiveOnlyFlags = errors.New("flags suffixed '-only' (i.e. --tls-auth-only and --config-only) are mutually exclusive")
)

type configInitOptions struct {
	// single configuration output switches (mutually exclusive)
	tlsAuthOnly          bool
	configOnly           bool
	vppConfigOnly        bool
	vppDayZeroConfigOnly bool

	// orchestrator related options
	vpnServerName string
	userName      string
	apiAddr       string
	basicAuth     bool

	// Required fields to build a file with TLS authentication data.
	// (+ orchestrator options are required as we need to connect to orchestrator to retrieve TLS auth data)
	tlsAuthPath string

	// Required fields to build a file with configuration.
	configPath        string
	grpcAddr          string
	grpcTimeout       int
	cacertPath        string
	caName            string
	reconnectAttempts int
	reconnectPause    int
	logLevel          string

	// VPP config/VPP day0 config related options
	useVPP               bool
	vppConfigPath        string
	vppDayZeroConfigPath string
	vppInterfaces        []string

	grpcRateLimiterTokensPerSecond float64
	grpcRateLimiterBucketSize      int

	// Healthcheck related options
	hcGRPCAddr                       string
	hcListenPort                     int
	hcGRPCRateLimiterTokensPerSecond float64
	hcGRPCRateLimiterBucketSize      int
}

type egvpnConfig struct {
	ServerName string `json:"serverName"`
}

func NewConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		newConfigInitCommand(),
	)

	return cmd
}

func newConfigInitCommand() *cobra.Command {
	var opts configInitOptions

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Creates files with configuration",
		Args:  cobra.NoArgs,
		Example: `
# If you use command without the arguments the tls certificates will be downloaded
# and config with default values will be created and will be saved to tls-auth.json and config.json files

egvpn config init

# If you already have a valid config or tls secrets you can use --tls-auth-only or --config-only,
# in this case config or certificates will not be created again

egvpn config init --tls-auth-only --tls-auth-path some/dir/cfg.json

# You can specify the config values you want to be customized

egvpn config init --grpc-timeout 10 --grpc-addr 172.18.0.3:8080 --common-name cn.com

# Also you can switch from JWT to basic auth

egvpn config init --basic-auth `,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigInit(opts)
		},
	}

	flags := cmd.Flags()

	// single config file generation switches (mutually exclusive)
	flags.BoolVar(&opts.tlsAuthOnly, "tls-auth-only", false, "Create only file with TLS auth certificates. Mutually exclusive with other '-only' suffixed flags.")
	flags.BoolVar(&opts.configOnly, "config-only", false, "Create only file with configuration. Mutually exclusive with other '-only' suffixed flags.")
	flags.BoolVar(&opts.vppConfigOnly, "vpp-config-only", false, "Creates only file with VPP configuration (vpp.conf). Mutually exclusive with other '-only' suffixed flags.")
	flags.BoolVar(&opts.vppDayZeroConfigOnly, "vpp-day0-config-only", false, "Creates only file with day0 configuration for VPP (vpp-day0.conf). Mutually exclusive with other '-only' suffixed flags.")

	// orchestrator related flags
	flags.StringVar(&opts.vpnServerName, "server-name", "", "Name of the EntGuard server. Will be prompted if not set")
	flags.BoolVar(&opts.basicAuth, "basic-auth", false, "Use basic authentication (JWT is by default)")
	flags.StringVar(&opts.userName, "user-name", "", "Name of the admin user. Required for retrieving TLS auth certificates and will be prompted if not set")
	flags.StringVar(&opts.apiAddr, "address", defaultAPIAddress, "Address of EG-O REST API server")

	// tls-auth config related flags
	flags.StringVar(&opts.tlsAuthPath, "tls-auth-path", defaultTLSAuthFileName, "Path to a file where TLS auth certificates should be stored")

	// server config related flags
	flags.StringVar(&opts.configPath, "config-path", defaultConfigFileName, "Path to a file where configuration should be stored")
	flags.StringVar(&opts.grpcAddr, "grpc-addr", defaultGRPCAddress, "Address of EG-O gRPC server for EntGuard (VPN) servers")
	flags.IntVar(&opts.grpcTimeout, "grpc-timeout", int(defaultGRPCTimeout.Seconds()), "Connection timeout for gRPC requests in seconds")
	flags.StringVar(&opts.caName, "ca-name", defaultCaName, "CA name is used for Server Name Indication (SNI) in TLS configuration")
	flags.StringVar(&opts.cacertPath, "ca-cert-path", defaultCacertFilePath, "Path to a file with root CA certificate")
	flags.IntVar(&opts.reconnectAttempts, "reconnect-attempts", defaultReconnectAttempts, "Number of attempts to restore connection with EG-O")
	flags.IntVar(&opts.reconnectPause, "reconnect-pause", int(defaultReconnectPause.Seconds()), "Pause in seconds between reconnecting attempts")
	flags.StringVar(&opts.logLevel, "log-level", defaultLogLevel, fmt.Sprintf("Log level of EG-S. Can be one of '%s', '%s', '%s'", log.WarnLevel.String(), log.InfoLevel.String(), log.DebugLevel.String()))
	flags.Float64Var(&opts.grpcRateLimiterTokensPerSecond, "grpc-ratelimiter-tps", defaultTokensPerSecond, "Tokens per second of token bucket-based rate limiting that rate limits handled GRPC requests by server")
	flags.IntVar(&opts.grpcRateLimiterBucketSize, "grpc-ratelimiter-bucketsize", defaultBucketSize, "Bucket size of token bucket-based rate limiting that rate limits handled GRPC requests by server")

	// VPP config/VPP day0 config related flags
	flags.BoolVar(&opts.useVPP, "use-vpp", false, "Creates also VPP specific configuration (vpp.conf and vpp-day0.conf)")
	flags.StringVar(&opts.vppConfigPath, "vpp-config-path", defaultVPPConfigFileName, "Path to a file where VPP config should be stored")
	flags.StringVar(&opts.vppDayZeroConfigPath, "vpp-day0-config-path", defaultVPPDayZeroConfigFileName, "Path to a file where VPP day0 config should be stored")
	flags.StringSliceVar(&opts.vppInterfaces, "vpp-interfaces", []string{}, "NIC PCIe addresses of interfaces that should VPP use from host (DPDK interfaces). These interfaces will not be available to host after egvpn start anymore, so plan accordingly")

	// healthcheck service config related flags
	flags.StringVar(&opts.hcGRPCAddr, "hc-grpc-addr", defaultHCGRPCAddress, "Address of EG-O gRPC server for healthcheck service")
	flags.IntVar(&opts.hcListenPort, "hc-listen-port", defaultHCListenPort, "Port on which the healthcheck service should listen for healthcheck pings")
	flags.Float64Var(&opts.hcGRPCRateLimiterTokensPerSecond, "hc-grpc-ratelimiter-tps", defaultHCTokensPerSecond, "Tokens per second of token bucket-based rate limiting that rate limits handled GRPC requests for healthcheck service")
	flags.IntVar(&opts.hcGRPCRateLimiterBucketSize, "hc-grpc-ratelimiter-bucketsize", defaultHCBucketSize, "Bucket size of token bucket-based rate limiting that rate limits handled GRPC requests for healthcheck service")

	return cmd
}

func runConfigInit(opts configInitOptions) error {
	// verifications
	if err := verifyConfigInitOptions(&opts); err != nil {
		return err
	}

	// config creation
	if opts.tlsAuthOnly || noSingleConfigCreation(&opts) {
		fmt.Println("Authentication is required.")
		username := opts.userName
		if username == "" {
			username = prompter.Prompt("Enter user name: ")
			if username == "" {
				return errEmptyUserName
			}
		}
		password := prompter.PromptNoEcho(fmt.Sprintf("Enter password for %s: ", username))
		if password == "" {
			return errEmptyPassword
		}
		apiClient, err := client.New(opts.apiAddr, opts.cacertPath)
		if err != nil {
			return err
		}
		if opts.basicAuth {
			apiClient.UseBasicAuth(username, password)
		} else {
			fmt.Printf("Authenticating on %s server as %q user.\n", opts.apiAddr, username)
			err = apiClient.UseJWTAuth(username, password)
			if err != nil {
				return fmt.Errorf("failed to authenticate the user %q with the given password: %v", username, err)
			}
		}
		fmt.Printf("Requesting TLS auth certificates for %q.\n", opts.vpnServerName)
		tlsAuth, err := apiClient.GetSecrets(opts.vpnServerName)
		if err != nil {
			return fmt.Errorf("REST API call failed: %v", err)
		}
		tlsAuthJSON, err := json.Marshal(tlsAuth)
		if err != nil {
			return fmt.Errorf("unable to marshal TLS auth to JSON: %w", err)
		}
		err = writeConfigContentToFile(opts.tlsAuthPath, tlsAuthJSON, 0400, "TLS auth")
		if err != nil {
			return fmt.Errorf("unable to save content to TLS auth file: %v", err)
		}
	}

	if opts.configOnly || noSingleConfigCreation(&opts) {
		fmt.Printf("Adding CA certificate to configuration file (CA path: %q).\n", opts.cacertPath)
		cacert, err := os.ReadFile(opts.cacertPath)
		if err != nil {
			return fmt.Errorf("failed to read CA certificate file: %v", err)
		}
		fmt.Printf("Generating configuration file for %q server.\n", opts.vpnServerName)
		content, err := json.MarshalIndent(config.Config{
			UseVPP:            opts.useVPP,
			RPCAddr:           opts.grpcAddr,
			RPCTimeout:        time.Duration(opts.grpcTimeout) * time.Second,
			CaName:            opts.caName,
			CaCert:            cacert,
			ReconnectAttempts: opts.reconnectAttempts,
			ReconnectPause:    time.Duration(opts.reconnectPause) * time.Second,
			LogLevel:          opts.logLevel,
			TokensPerSecond:   opts.grpcRateLimiterTokensPerSecond,
			BucketSize:        opts.grpcRateLimiterBucketSize,
			HCGRPCAddr:        opts.hcGRPCAddr,
			HCListenPort:      opts.hcListenPort,
			HCTokensPerSecond: opts.hcGRPCRateLimiterTokensPerSecond,
			HCBucketSize:      opts.hcGRPCRateLimiterBucketSize,
		}, "", "    ")
		if err != nil {
			return fmt.Errorf("failed to generate configuration: %v", err)
		}
		err = writeConfigContentToFile(opts.configPath, content, 0644, "configuration")
		if err != nil {
			return fmt.Errorf("unable to save content to configuration file: %v", err)
		}
	}
	if opts.vppConfigOnly || (noSingleConfigCreation(&opts) && opts.useVPP) {
		content := generateVPPConfig(opts.vppInterfaces)
		err := writeConfigContentToFile(opts.vppConfigPath, []byte(content), 0644, "vpp configuration")
		if err != nil {
			return fmt.Errorf("unable to save content to VPP configuration file: %v", err)
		}
	}
	if opts.vppDayZeroConfigOnly || (noSingleConfigCreation(&opts) && opts.useVPP) {
		err := writeConfigContentToFile(opts.vppDayZeroConfigPath,
			[]byte(vppDayZeroConfig), 0644, "VPP day0 configuration")
		if err != nil {
			return fmt.Errorf("unable to save content to VPP day0 configuration file: %v", err)
		}
	}

	// internal save of information just for this tool
	err := saveEgvpnConfig(egvpnConfig{ServerName: opts.vpnServerName})
	if err != nil {
		return fmt.Errorf("failed to save server name: %v", err)
	}

	// usage suggestions
	if noSingleConfigCreation(&opts) {
		if opts.useVPP {
			checkVPPInterfacesPresence(&opts)
			fmt.Printf("You can start VpnS using this command:\n\tsudo egvpn start -t %v -c %v -p %v -z %v\n", opts.tlsAuthPath, opts.configPath, opts.vppConfigPath, opts.vppDayZeroConfigPath)
		} else {
			fmt.Printf("You can start VpnS using this command:\n\tsudo egvpn start -t %v -c %v\n", opts.tlsAuthPath, opts.configPath)
		}
	}

	return nil
}

func checkVPPInterfacesPresence(opts *configInitOptions) {
	for _, pciAddress := range opts.vppInterfaces {
		pathChecked := "/sys/bus/pci/devices/" + pciAddress // every PCI card must have this directory in linux
		_, err := os.Stat(pathChecked)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Printf("Warning: don't run VpnS on current system because NIC PCI address(%s) "+
					"for interface that should be used in VPP doesn't exist in current system (checked "+
					"for existence of path %s): %v\n", pciAddress, pathChecked, err)
				continue
			}
			fmt.Printf("Warning: checking presence of VPP interface(%s) on current system failed: "+
				"checking for existence of %s in filesystem failed (do you run command as root?): %v  (Warning can "+
				"be ignored if the current system is not the system where VpnS will be started)\n",
				pciAddress, pathChecked, err)
		}
	}
}

func writeConfigContentToFile(configPath string, content []byte, filemode os.FileMode, contentDescription string) error {
	fmt.Printf("Saving %s file to %q\n", contentDescription, configPath)
	err := os.MkdirAll(filepath.Dir(configPath), 0775)
	if err != nil {
		return fmt.Errorf("unable to create directories: %v", err)
	}
	err = os.WriteFile(configPath, content, filemode)
	if err != nil {
		return fmt.Errorf("unable to save to the file: %v", err)
	}
	return nil
}

func noSingleConfigCreation(opts *configInitOptions) bool {
	return !opts.tlsAuthOnly && !opts.configOnly && !opts.vppConfigOnly && !opts.vppDayZeroConfigOnly
}

func verifyConfigInitOptions(opts *configInitOptions) error {
	if !areSwitchesMutualExclusive(opts.tlsAuthOnly, opts.configOnly, opts.vppConfigOnly, opts.vppDayZeroConfigOnly) {
		return errMutualExclusiveOnlyFlags
	}

	var err error

	if opts.tlsAuthOnly || noSingleConfigCreation(opts) {
		err = verifyTLSAuthPath(opts)
		if err != nil {
			return err
		}
	}

	if opts.configOnly || noSingleConfigCreation(opts) {
		err = verifyConfigPath(opts)
		if err != nil {
			return err
		}
	}

	if opts.vppConfigOnly || (noSingleConfigCreation(opts) && opts.useVPP) {
		err = verifyVPPConfigPath(opts)
		if err != nil {
			return err
		}
	}

	if opts.vppDayZeroConfigOnly || (noSingleConfigCreation(opts) && opts.useVPP) {
		err = verifyVPPDayZeroConfigPath(opts)
		if err != nil {
			return err
		}
	}

	if err = verifyCacertPath(opts); err != nil {
		return err
	}

	err = verifyServerNameOption(opts)
	if err != nil {
		return err
	}

	err = verifyLogLevel(opts)
	if err != nil {
		return err
	}

	return nil
}

func verifyServerNameOption(opts *configInitOptions) error {
	if opts.vpnServerName != "" {
		return nil
	}

	input := prompter.Prompt("Enter EntGuard server name: ")
	if input == "" {
		return errEmptyServerName
	}
	opts.vpnServerName = input

	return nil
}

func verifyTLSAuthPath(opts *configInitOptions) error {
	_, err := os.Stat(opts.tlsAuthPath)
	if err == nil {
		return fmt.Errorf("the file for TLS auth certificates already exists. Please, remove it and start again (path: %q)", opts.tlsAuthPath)
	}
	return nil
}

func verifyConfigPath(opts *configInitOptions) error {
	_, err := os.Stat(opts.configPath)
	if err == nil {
		return fmt.Errorf("the configuration file already exists. Please, remove it and start again (path: %q)", opts.configPath)
	}
	return nil
}

func verifyVPPConfigPath(opts *configInitOptions) error {
	_, err := os.Stat(opts.vppConfigPath)
	if err == nil {
		return fmt.Errorf("the VPP configuration file already exists. Please, remove it and start again (path: %q)", opts.vppConfigPath)
	}
	return nil
}

func verifyVPPDayZeroConfigPath(opts *configInitOptions) error {
	_, err := os.Stat(opts.vppDayZeroConfigPath)
	if err == nil {
		return fmt.Errorf("the VPP day0 configuration file already exists. Please, remove it and start again (path: %q)", opts.vppDayZeroConfigPath)
	}
	return nil
}

func verifyCacertPath(opts *configInitOptions) error {
	_, err := os.Stat(opts.cacertPath)
	if err != nil {
		return fmt.Errorf("could not find CA certificate file using specified path (path: %q)", opts.cacertPath)
	}
	return nil
}

func verifyLogLevel(opts *configInitOptions) error {
	_, err := log.ParseLevel(opts.logLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %q", opts.logLevel)
	}
	return nil
}

func saveEgvpnConfig(cfg egvpnConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("unable to marshal egvpn config: %v", err)
	}

	err = os.MkdirAll(filepath.Dir(egvpnConfigPath), 0775)
	if err != nil {
		return fmt.Errorf("unable to create egvpn config directories: %v", err)
	}

	err = os.WriteFile(egvpnConfigPath, data, 0664)
	if err != nil {
		return fmt.Errorf("unable to create egvpn config file: %v", err)
	}

	return nil
}

func loadEgvpnConfig() (*egvpnConfig, error) {
	data, err := os.ReadFile(egvpnConfigPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read egvpn config file: %v", err)
	}

	var cfg egvpnConfig
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal egvpn config data: %v", err)
	}

	return &cfg, nil
}

func areSwitchesMutualExclusive(switchVars ...bool) bool {
	counter := 0
	for _, switchVar := range switchVars {
		if switchVar {
			counter++
		}
	}
	return counter < 2
}

func generateVPPConfig(vppInterfaces []string) string {
	base := `# example config with more settings can be found at https://github.com/FDio/vpp/blob/stable/2406/src/vpp/conf/startup.conf (it is not full list of settings as i.e. VPP plugins can have additional settings)
unix {
  nodaemon
  log /var/log/vpp/vpp.log
  full-coredump
  cli-listen /run/vpp/cli.sock
  gid vpp
}

api-trace {
  on
}

api-segment {
  gid vpp
}

socksvr {
  default
}
`
	disableDpdk := `
plugins {
	plugin dpdk_plugin.so { disable }
}
`

	var config strings.Builder
	config.WriteString(base)
	if len(vppInterfaces) == 0 {
		// no dpdk interfaces to use -> disable dpdk plugin to not automatically take some interfaces and/or
		// run dpdk probing for traffic to handle (1 core 100% usage)
		config.WriteString(disableDpdk)
		return config.String()
	}

	// write dpdk interface (there is at least one)
	config.WriteString("dpdk {\n")
	for i, pciAddress := range vppInterfaces {
		fmt.Fprintf(&config, "\tdev %s {\n\t\tname dpdk%d\n\t}\n", pciAddress, i+1)
	}
	config.WriteString("}\n")

	return config.String()
}
