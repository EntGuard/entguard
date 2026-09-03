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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.fd.io/govpp"
	"go.fd.io/govpp/binapi/vlib"
	"go.fd.io/govpp/core"

	"github.com/entguard/entguard/cmd/egvpn/buildinfo"
	config "github.com/entguard/entguard/pkg/server-config"
)

const (
	defaultConfigFilePath     = "/etc/vpn-s/config.json"
	defaultSecretsFilePath    = "/etc/vpn-s/secrets.json"
	defaultVPPConfigFilePath  = "/etc/vpp/startup.conf"
	defaultTelegrafConfigPath = "/etc/telegraf/telegraf.conf"

	defaultVPPConnectionCheckTimeout = 60
	defaultVPPHugepages2MCount       = 512 // 512 * 2MiB = 1GiB of memory
	dontCheckAndSetVPPHugepages      = -1  // value for vpp-hugepages-2m-count flag to not set/check hugepages

	telegrafVolumeName          = "sockets"
	telegrafVolumeMountPath     = "/run/sockets"
	vppSocketDirVolumeName      = "eg-vpp-socket-dir"
	vppSocketDirVolumeMountPath = "/run/vpp/"

	vppAPISocketFileName = "api.sock"

	egvpnRestartOptsPath = "/etc/entguard/egvpn-restart-opts.json"

	hugepagesCountKernelSettingName      = "vm.nr_hugepages"
	maxMapCountKernelSettingName         = "vm.max_map_count"
	hugepageUsageFilterKernelSettingName = "vm.hugetlb_shm_group"
	maxSharedMemoryKernelSettingName     = "kernel.shmmax"
)

var (
	errWireguardNotInstalled    = errors.New("wireguard is not installed")
	errDockerNotInstalled       = errors.New("docker is not installed")
	errRunningContainer         = errors.New("failed to run a container, is it already running?")
	errSecretsNotFound          = errors.New("secrets was not found in the directory")
	errConfigNotFound           = errors.New("config was not found in the directory")
	errVPPConfigNotFound        = errors.New("vpp config was not found in the directory")
	errVPPDayZeroConfigNotFound = errors.New("vpp day0 config was not found in the directory")
	errCurrentDir               = errors.New("failed to get current directory")
)

type egVPNOptions struct {
	SecretsPath          string `json:"secretsPath"`
	ConfigPath           string `json:"configPath"`
	VPPConfigPath        string `json:"VPPConfigPath"`
	VPPDayZeroConfigPath string `json:"VPPDayZeroConfigPath"`

	EnableTelemetry    bool   `json:"enableTelemetry"`
	TelegrafConfigPath string `json:"telegrafConfigPath"`
	TelegrafServerName string `json:"telegrafServerName"`
	TelegrafVersion    string `json:"TelegrafVersion"`

	VPPConnectionCheckTimeout int `json:"VPPConnectionCheckTimeout"`

	HugePages2MCount int `json:"HugePages2MCount"`

	Service ServiceType `json:"service"`
}

func NewStartCommand() *cobra.Command {
	var opts egVPNOptions

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Starts EntGuard server and Healthcheck containers",
		Long: `Starts EntGuard server container from ` + buildinfo.EgServerImageName() + ` image` +
			` and Healthcheck container from ` + buildinfo.EgHealthcheckImageName() + ` image.` +
			` Optionally starts Telegraf and VPP containers and creates docker volumes for telemetry and VPP sockets.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.SecretsPath, "tls-auth-path", "t", defaultTLSAuthFileName, "path to the tls-auth-path file")
	flags.StringVarP(&opts.ConfigPath, "config-path", "c", defaultConfigFileName, "path to the config file")
	flags.StringVarP(&opts.VPPConfigPath, "vpp-config-path", "p", defaultVPPConfigFileName, "path to the VPP config file")
	flags.StringVarP(&opts.VPPDayZeroConfigPath, "vpp-day0-config-path", "z", defaultVPPDayZeroConfigFileName, "path to the VPP day0 config file")

	flags.BoolVarP(&opts.EnableTelemetry, "enable-telemetry", "e", false, "if set, the Telegraf container will be started. The telemetry will be sent only if it is also an enabled feature in EG-O")
	flags.StringVarP(&opts.TelegrafConfigPath, "tel-config", "m", "", "path to the Telegraf config file")
	flags.StringVarP(&opts.TelegrafServerName, "server-name", "n", "", "server name that will be shown in Grafana. It will overwrite the name in the egvpn config")
	flags.StringVarP(&opts.TelegrafVersion, "tel-version", "v", "", "Telegraf image version, if not set will be used the default one")

	flags.IntVarP(&opts.VPPConnectionCheckTimeout, "vpp-connection-check-timeout", "o", defaultVPPConnectionCheckTimeout, "Timeout (in seconds) for checking connection to VPP using VPP socket. Connection is also used for applying day0 config to VPP.")

	flags.IntVarP(&opts.HugePages2MCount, "vpp-hugepages-2m-count", "u", defaultVPPHugepages2MCount, fmt.Sprintf("Count of 2MiB hugepages that should be checked for and allocated for VPP usage. (default is %d, using %d will disable check and dynamic allocation)", defaultVPPHugepages2MCount, dontCheckAndSetVPPHugepages))

	opts.Service = AllServiceType
	flags.VarP(&opts.Service, "service", "s", fmt.Sprintf("service to start. ServiceType: %s|%s|%s", VPNServerServiceType, HealthCheckServiceType, AllServiceType))

	return cmd
}

func runStart(opts egVPNOptions) error {
	// verification (and serverCfg retrieval)
	if err := verifyServerConfigStartOption(&opts); err != nil {
		return err
	}
	serverCfg, err := config.NewConfigFromFile(opts.ConfigPath, nil)
	if err != nil {
		return fmt.Errorf("failed to read server config from file %q: %w", opts.ConfigPath, err)
	}
	if err := verifyInstallation(serverCfg.UseVPP); err != nil {
		return err
	}
	if err := verifyOtherServerStartOptions(&opts, serverCfg.UseVPP); err != nil {
		return err
	}

	// remember start options for possible restart
	if err := saveRestartOpts(&opts); err != nil {
		return err
	}

	// create needed volumes and run all containers
	currentDir, err := os.Getwd()
	if err != nil {
		return errCurrentDir
	}

	if opts.Service == VPNServerServiceType ||
		opts.Service == AllServiceType {
		if err := runServerContainer(&opts, currentDir, serverCfg.UseVPP); err != nil {
			return err
		}

		if serverCfg.UseVPP {
			if err := prepareVPPSocketDirVolume(); err != nil {
				return err
			}
			if err := runVPPContainer(&opts, currentDir); err != nil {
				return err
			}
			goVPPConn, err := waitForVPPToBeReadyForConnection(&opts)
			if err != nil {
				return err
			}
			defer goVPPConn.Disconnect() // cleanup of resources
			if err := applyVPPDayZeroConfig(&opts, goVPPConn); err != nil {
				return err
			}
		}
		if opts.EnableTelemetry {
			if err := prepareTelemetryVolume(); err != nil {
				return err
			}
			if err := runTelegrafContainer(&opts, currentDir); err != nil {
				return err
			}
		}
	}

	if opts.Service == HealthCheckServiceType ||
		opts.Service == AllServiceType {
		if err := runHealthcheckContainer(&opts, currentDir, serverCfg.HCListenPort); err != nil {
			return err
		}
	}

	return nil
}

func waitForVPPToBeReadyForConnection(opts *egVPNOptions) (*core.Connection, error) {
	fmt.Print("Waiting for VPP to be able to connect to it")

	// get VPP API socket directory on host (where the volume is located on host)
	output, err := exec.Command("docker", "volume", "inspect", "--format={{.Mountpoint}}",
		vppSocketDirVolumeName).CombinedOutput()
	if err != nil {
		fmt.Println() // just formatting to get error on new line
		return nil, fmt.Errorf("failed to find VPP socket on host: %w", err)
	}
	vppAPISocketDirOnHost := strings.TrimSpace(string(output))

	// silence govpp (it keeps log warnings into the waiting text of dots)
	core.SetLogLevel(log.PanicLevel)

	// do async connect to VPP
	conn, connEv, err := govpp.AsyncConnect(filepath.Join(vppAPISocketDirOnHost, vppAPISocketFileName),
		int((time.Duration(opts.VPPConnectionCheckTimeout)*time.Second)/core.DefaultReconnectInterval),
		core.DefaultReconnectInterval)
	if err != nil {
		fmt.Println() // just formatting to get error on new line
		return nil, fmt.Errorf("failed to start async connecting to VPP: %v", err)
	}

	// waiting for govpp to connect to VPP
	for {
		select {
		case e := <-connEv: // Note: timeout of async connect = get event with bad state
			if e.State != core.Connected {
				fmt.Println() // just formatting to get error on new line
				return nil, fmt.Errorf("failed to connect to VPP (received connection event %+v)", e)
			}
			fmt.Println("\nDone.")
			return conn, nil
		case <-time.After(core.DefaultReconnectInterval): // this is just for console output progress
			fmt.Print(".")
		}
	}
}

func applyVPPDayZeroConfig(opts *egVPNOptions, goVPPConn *core.Connection) error {
	fmt.Print("Applying VPP day0 config")

	// open config file
	configFile, err := os.Open(opts.VPPDayZeroConfigPath)
	if err != nil {
		return fmt.Errorf("can't open VPP day0 config file %s: %w", opts.VPPDayZeroConfigPath, err)
	}
	defer func() { _ = configFile.Close() }()

	// read and apply config line by line (1 line = 1 VPP CLI command)
	vppClient := vlib.NewServiceClient(goVPPConn)
	fileScanner := bufio.NewScanner(configFile)
	fileScanner.Split(bufio.ScanLines)
	for fileScanner.Scan() {
		vppCLICmd := strings.TrimSpace(fileScanner.Text())
		if len(vppCLICmd) == 0 {
			continue // ignore empty lines
		}
		if strings.HasPrefix(vppCLICmd, "#") {
			continue // ignore comment lines
		}

		// apply 1 VPP CLI command
		// (Note: this could not be done using vppctl tool using exec.Command("docker", "exec", "-t",
		// vppContainerName, "vppctl", vppCLICmd), because return code of every failure to apply config in VPP
		// is lost somewhere and zero return code is always returned (error output of executed command is
		// also no help as error is put into standard output and can't be easily distinguished by normal
		// vpp command output)
		_, err = vppClient.CliInband(context.Background(), &vlib.CliInband{Cmd: vppCLICmd})
		if err != nil {
			fmt.Println() // just formatting to get error on new line
			return fmt.Errorf("failed to apply VPP CLI command \"%s\": %w ", vppCLICmd, err)
		}
		fmt.Print(".")
	}
	fmt.Println("\nDone.")

	return nil
}

func prepareVPPSocketDirVolume() error {
	fmt.Println("Creating docker volume for VPP socket")
	if err := exec.Command("docker", "volume", "create", vppSocketDirVolumeName).Run(); err != nil {
		return errors.New("failed to create docker volume for VPP socket")
	}
	fmt.Println("Volume for VPP socket was created")
	return nil
}

func prepareTelemetryVolume() error {
	fmt.Println("Creating docker volume for telemetry socket")
	if err := exec.Command("docker", "volume", "create", telegrafVolumeName).Run(); err != nil {
		return errors.New("failed to create docker volume for telemetry socket")
	}
	fmt.Println("Volume for telemetry socket was created")
	return nil
}

func runVPPContainer(opts *egVPNOptions, currentDir string) error {
	cmd := []string{
		"run", "-d",
		"--volume", computeFileMountParameter(opts.VPPConfigPath, defaultVPPConfigFilePath, currentDir),
		"--volume", vppSocketDirVolumeName + ":" + vppSocketDirVolumeMountPath,
		"--privileged",
		"--name", vppContainerName, buildinfo.EgVPPImageName(),
	}

	fmt.Println("Starting the VPP container")
	if err := exec.Command("docker", cmd...).Run(); err != nil {
		return errRunningContainer
	}
	fmt.Println("VPP container is running.")

	return nil
}

func computeFileMountParameter(hostFilePath, containerFilePath, currentDir string) string {
	if filepath.IsAbs(hostFilePath) {
		return hostFilePath + ":" + containerFilePath
	}
	return filepath.Join(currentDir, hostFilePath) + ":" + containerFilePath
}

func runServerContainer(opts *egVPNOptions, currentDir string, useVPP bool) error {
	cmd := []string{
		"run", "-d",
		"--volume", computeFileMountParameter(opts.SecretsPath, defaultSecretsFilePath, currentDir),
		"--volume", computeFileMountParameter(opts.ConfigPath, defaultConfigFilePath, currentDir),
	}
	if useVPP {
		cmd = append(cmd, "--volume", vppSocketDirVolumeName+":"+vppSocketDirVolumeMountPath)
	}
	if opts.EnableTelemetry {
		cmd = append(cmd, "--volume", telegrafVolumeName+":"+telegrafVolumeMountPath)
	}
	cmd = append(cmd,
		"--cap-add=NET_ADMIN",
		"--network", "host",
		"--name", serverContainerName, buildinfo.EgServerImageName(),
		"/vpn_server", "--secrets", defaultSecretsFilePath, "--config", defaultConfigFilePath,
	)
	if opts.EnableTelemetry {
		cmd = append(cmd, "--enable-telemetry")
	}

	fmt.Printf("Starting EntGuard server container with name %s\n", serverContainerName)
	if err := exec.Command("docker", cmd...).Run(); err != nil {
		return errRunningContainer
	}
	fmt.Println("EntGuard server container is running.")

	return nil
}

func runHealthcheckContainer(opts *egVPNOptions, currentDir string, listenPort int) error {
	cmd := []string{
		"run", "-d",
		"-p", fmt.Sprintf("%d:%d", listenPort, listenPort),
		"--volume", computeFileMountParameter(opts.SecretsPath, defaultSecretsFilePath, currentDir),
		"--volume", computeFileMountParameter(opts.ConfigPath, defaultConfigFilePath, currentDir),

		//TODO: after healthcheck ping responder is implemented check if capability and network are really needed here
		"--cap-add=NET_ADMIN",
		"--network", "host",

		"--name", healthcheckContainerName, buildinfo.EgHealthcheckImageName(),
		"/healthcheck", "--secrets", defaultSecretsFilePath, "--config", defaultConfigFilePath,
	}

	fmt.Printf("Starting EntGuard healthcheck container with name %s\n", healthcheckContainerName)
	if err := exec.Command("docker", cmd...).Run(); err != nil {
		return errRunningContainer
	}
	fmt.Println("EntGuard healthcheck container is running.")
	return nil
}

func runTelegrafContainer(opts *egVPNOptions, currentDir string) error {
	if opts.TelegrafConfigPath != "" {
		fmt.Println("Checking if user provided config is present.")
		if _, err := os.Stat(opts.TelegrafConfigPath); os.IsNotExist(err) {
			fmt.Println("Telegraf config was not found. Telegraf will not start")
			return errConfigNotFound
		}
	} else {
		fmt.Println("Path for Telegraf config is not provided. Using default config.")
		temp, err := os.CreateTemp("", "telegraf")
		if err != nil {
			return fmt.Errorf("failed to create temporary config file: %v", err)
		}
		defer func() { _ = os.Remove(temp.Name()) }()

		_, err = temp.Write([]byte(telegrafConfig))
		if err != nil {
			return fmt.Errorf("failed to write to the temporary config file: %v", err)
		}
		opts.TelegrafConfigPath = temp.Name()
	}

	if opts.TelegrafServerName == "" {
		fmt.Println("Server name was not provided. Loading it.")
		cfg, err := loadEgvpnConfig()
		if err != nil {
			return err
		}
		opts.TelegrafServerName = cfg.ServerName
	}

	// sleep to give eg-s container time to create telemetry socket before telegraf container starts
	time.Sleep(1 * time.Second)
	fmt.Printf("Starting Telegraf container with server name: %s\n", opts.TelegrafServerName)
	err := exec.Command(
		"docker", "run", "-d",
		"-v", telegrafVolumeName+":"+telegrafVolumeMountPath,
		"--cap-add=NET_ADMIN",
		"--network", "host",
		"-e", "VPN_SERVER_NAME="+opts.TelegrafServerName,
		"--volume", computeFileMountParameter(opts.TelegrafConfigPath, defaultTelegrafConfigPath, currentDir),
		"--entrypoint", "telegraf",
		"--name", telegrafContainerName, buildinfo.EgTelegrafImageName(opts.TelegrafVersion),
	).Run()

	if err != nil {
		return errRunningContainer
	}

	fmt.Println("Telegraf container is running")
	return nil
}

func verifyOtherServerStartOptions(opts *egVPNOptions, useVPP bool) error {
	fmt.Println("Check if secrets are present")
	if _, err := os.Stat(opts.SecretsPath); os.IsNotExist(err) {
		return errSecretsNotFound
	}
	fmt.Println("Done.")

	if useVPP {
		fmt.Println("Check if VPP config is present")
		if _, err := os.Stat(opts.VPPConfigPath); os.IsNotExist(err) {
			return errVPPConfigNotFound
		}
		fmt.Println("Done.")

		fmt.Println("Check if VPP day0 config is present")
		if _, err := os.Stat(opts.VPPDayZeroConfigPath); os.IsNotExist(err) {
			return errVPPDayZeroConfigNotFound
		}
		fmt.Println("Done.")

		if err := verifyAndAllocateHugePages(opts); err != nil {
			return err
		}
	}

	return nil
}

func verifyServerConfigStartOption(opts *egVPNOptions) error {
	fmt.Println("Check if server config is present")
	if _, err := os.Stat(opts.ConfigPath); os.IsNotExist(err) {
		return errConfigNotFound
	}
	fmt.Println("Done.")
	return nil
}

func verifyImageExists(name string) error {
	fmt.Printf("Checking if %s image is present\n", name)
	if err := exec.Command("docker", "inspect", name).Run(); err != nil {
		return fmt.Errorf("image %s not found", name)
	}
	fmt.Println("Done.")
	return nil
}

func verifyInstallation(useVPP bool) error {
	if !useVPP { // only linux wireguard backend uses linux tool called "wg"
		fmt.Println("Checking if WireGuard is installed")
		if err := exec.Command("sudo", "wg").Run(); err != nil {
			return errWireguardNotInstalled
		}
		fmt.Println("Done.")
	}

	fmt.Println("Checking if docker is installed")
	if err := exec.Command("docker").Run(); err != nil {
		return errDockerNotInstalled
	}
	fmt.Println("Done.")

	if err := verifyImageExists(buildinfo.EgServerImageName()); err != nil {
		return err
	}
	if err := verifyImageExists(buildinfo.EgHealthcheckImageName()); err != nil {
		return err
	}

	return nil
}

func verifyAndAllocateHugePages(opts *egVPNOptions) error {
	if opts.HugePages2MCount < 0 { // covers dontCheckAndSetVPPHugepages, but also other invalid negative values
		fmt.Println("Checking/Changing of Hugepages skipped.")
		return nil
	}

	// checking hugepages
	fmt.Println("Checking Hugepages setting.")
	allocatedHP, err := allocatedHugePages()
	if err != nil {
		return fmt.Errorf("failed to retrieve allocated hugepages count: %w", err)
	}
	if allocatedHP < opts.HugePages2MCount {
		if err := resizeHugePages(opts.HugePages2MCount); err != nil {
			return err
		}
		fmt.Printf("Kernel setting(%s) set to %d.\n", hugepagesCountKernelSettingName, opts.HugePages2MCount)
	} else {
		fmt.Println("Setting is OK.")
	}

	// Note: Older VPP documentation (https://fd.io/docs/vpp/v2101/gettingstarted/users/configuring/hugepages.html)
	// suggest to also fix other kernel settings (can't find hugepages doc page for newer VPP version, but
	// the recommendation should be the same)

	// checking max map count
	fmt.Printf("Checking Hugepages related kernel setting(%s).\n", maxMapCountKernelSettingName)
	maxMapCount, err := readIntKernelSetting(maxMapCountKernelSettingName)
	if err != nil {
		return fmt.Errorf("failed to read kernel setting(%s): %w", maxMapCountKernelSettingName, err)
	}
	if int(maxMapCount) < (2 * opts.HugePages2MCount) {
		if err := writeIntKernelSetting(maxMapCountKernelSettingName, uint64(2*opts.HugePages2MCount)); err != nil {
			return fmt.Errorf("failed to set kernel setting(%s) to %d: %w",
				maxMapCountKernelSettingName, 2*opts.HugePages2MCount, err)
		}
		fmt.Printf("Kernel setting(%s) set to %d.\n", maxMapCountKernelSettingName, 2*opts.HugePages2MCount)
	} else {
		fmt.Println("Setting is OK.")
	}

	// checking hugepage usage filtering (there is filter in kernel preventing to use hugepages by some processes)
	fmt.Printf("Checking Hugepages related kernel setting(%s).\n", hugepageUsageFilterKernelSettingName)
	groupID, err := readIntKernelSetting(hugepageUsageFilterKernelSettingName)
	if err != nil {
		return fmt.Errorf("failed to read kernel setting(%s): %w", hugepageUsageFilterKernelSettingName, err)
	}
	if groupID != 0 { // -> set it to 0 so that any user group can use hugepages
		if err := writeIntKernelSetting(hugepageUsageFilterKernelSettingName, 0); err != nil {
			return fmt.Errorf("failed to set kernel setting(%s) to 0: %w",
				hugepageUsageFilterKernelSettingName, err)
		}
		fmt.Printf("Kernel setting(%s) set to 0.\n", hugepageUsageFilterKernelSettingName)
	} else {
		fmt.Println("Setting is OK.")
	}

	// checking max shared memory (can limit hugepages usage)
	fmt.Printf("Checking Hugepages related kernel setting(%s).\n", maxSharedMemoryKernelSettingName)
	maxSharedMemory, err := readIntKernelSetting(maxSharedMemoryKernelSettingName)
	if err != nil {
		return fmt.Errorf("failed to read kernel setting(%s): %w", maxSharedMemoryKernelSettingName, err)
	}
	minimalMaxSharedMemory := uint64(opts.HugePages2MCount) * 2 * 1024 * 1024
	if maxSharedMemory < minimalMaxSharedMemory {
		if err := writeIntKernelSetting(maxSharedMemoryKernelSettingName, minimalMaxSharedMemory); err != nil {
			return fmt.Errorf("failed to set kernel setting(%s) to %d: %w",
				maxSharedMemoryKernelSettingName, minimalMaxSharedMemory, err)
		}
		fmt.Printf("Kernel setting(%s) set to %d.\n", maxSharedMemoryKernelSettingName, minimalMaxSharedMemory)
	} else {
		fmt.Println("Setting is OK.")
	}

	return nil
}

func readIntKernelSetting(name string) (uint64, error) {
	out, err := exec.Command("sysctl", name, "-n").CombinedOutput()
	if err != nil {
		return 0, err
	}
	// Note: we need range more then int -> not using strconv.Atoi
	value, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 0)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func writeIntKernelSetting(name string, value uint64) error {
	return exec.Command("sysctl", "-w", fmt.Sprintf("%s=%d", name, value)).Run()
}

func allocatedHugePages() (int, error) {
	hp, err := readIntKernelSetting(hugepagesCountKernelSettingName)
	return int(hp), err
}

func resizeHugePages(size int) error {
	const hugePageSize = 2048
	err := writeIntKernelSetting(hugepagesCountKernelSettingName, uint64(size))
	if err != nil {
		return err
	}
	allocatedHP, err := allocatedHugePages()
	if err != nil {
		return err
	}

	if size != allocatedHP {
		return fmt.Errorf("failed to allocate enough hugepages (%d),currently allocated "+
			"%d hugepages, totally continuous memory %d MB.\nTo resolve the hugepages needed to run the VPP, "+
			"you can:\n"+
			"\t1.manually change the boot parameters for host to allocate hugepages and reboot (permanent solution).\n"+
			"\t2. reboot and try to set hugepages right after boot \"sysctl -w %s=%d\" (temporal solution until "+
			"restart, unless you do it at each boot)\n"+
			"\t3.change hugepage parameter that is used to check hugepages at start and change VPP config's memory "+
			"setting accordingly (Warning: lower memory for VPP can lead to out-of-memory problems)\n"+
			"\t4.disable hugepage checking/settings (and other kernel settings,see %s) by setting hugepage "+
			"parameter to %d and handle enough hugepages manually",
			size, allocatedHP, (allocatedHP*hugePageSize)/1000, hugepagesCountKernelSettingName, size,
			"https://fd.io/docs/vpp/v2101/gettingstarted/users/configuring/hugepages.html", dontCheckAndSetVPPHugepages)
	}

	return nil
}

func saveRestartOpts(opts *egVPNOptions) error {
	data, err := json.Marshal(*opts)
	if err != nil {
		return fmt.Errorf("unable to marshal restart options: %v", err)
	}

	err = os.MkdirAll(filepath.Dir(egvpnRestartOptsPath), 0775)
	if err != nil {
		return fmt.Errorf("unable to create restart options directories: %v", err)
	}

	err = os.WriteFile(egvpnRestartOptsPath, data, 0664)
	if err != nil {
		return fmt.Errorf("unable to create restart options file: %v", err)
	}

	return nil
}

func loadRestartOpts() (*egVPNOptions, error) {
	data, err := os.ReadFile(egvpnRestartOptsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read restart options file: %v, have you started it?", err)
	}

	var opts egVPNOptions
	err = json.Unmarshal(data, &opts)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal restart options data: %v", err)
	}

	return &opts, nil
}
