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
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const failedReportFilename = "_failed-reports.txt"

func NewReportCommand() *cobra.Command {
	var opts ReportOptions
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Create running state report",
		Long: "Create reports about running software stack (EntGuard Server, EntGuard Healthcheck, EntGuard Orchestrator, PostgreSQL, ...) " +
			"to allow quicker resolving of problems. The report will be a zip file containing " +
			"information split into multiple files.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.OutputDirectory, "output-directory", "o", "",
		"Output directory (as absolute path) where report zip file will be written. "+
			"Default is current directory.")
	flags.BoolVarP(&opts.ExcludeSecrets, "exclude-secrets", "e", true,
		"Exclude sensitive data (passwords, private and pre-shared keys) from the report.")

	return cmd
}

type ReportOptions struct {
	OutputDirectory string
	ExcludeSecrets  bool
}

func runReport(opts ReportOptions) error {
	// create report time and dependent variables
	reportTime := time.Now()
	reportName := fmt.Sprintf("egvpn-report--%s",
		strings.ReplaceAll(reportTime.UTC().Format("2006-01-02--15-04-05-.000"), ".", ""))

	// create temporal directory
	dirNamePattern := fmt.Sprintf("%v--*", reportName)
	dirName, err := os.MkdirTemp("", dirNamePattern)
	if err != nil {
		return fmt.Errorf("can't create tmp directory with name pattern %s: %w", dirNamePattern, err)
	}
	defer func() { _ = os.RemoveAll(dirName) }()

	results := []error{
		writeDockerLogsTo("vpn-server-logs.txt", dirName, serverContainerName),
		writeDockerLogsTo("healthcheck-logs.txt", dirName, healthcheckContainerName),
		writeDockerLogsTo("orchestrator-logs.txt", dirName, "eg-orchestrator"),
		writeDockerLogsTo("postgres-logs.txt", dirName, "eg-postgres"),
		copyDockerFileTo("config.json", dirName, "eg-s:/etc/vpn-s/config.json"),
		dumpDatabaseTo("database.sql", dirName, "eg-postgres", "entguard", "postgres", opts.ExcludeSecrets),
		writeCmdOutputTo("docker-ps.txt", dirName, "docker ps -a"),
		writeCmdOutputTo("ip-addr.txt", dirName, "ip addr"),
		writeCmdOutputTo("system-info.txt", dirName, "uname -a; echo '---'; cat /etc/os-release 2>/dev/null; echo '---'; free -h; echo '---'; df -h"),
	}

	if isVPPUsageDetected() {
		results = append(results,
			writeDockerLogsTo("vpp-logs.txt", dirName, vppContainerName),
			writeCmdOutputTo("wg-show.txt", dirName,
				"docker exec "+vppContainerName+" vppctl show wireguard interface"+
					"; echo '---'; docker exec "+vppContainerName+" vppctl show wireguard peer"+
					"; echo '---'; docker exec "+vppContainerName+" vppctl show interface"),
		)
	} else {
		results = append(results,
			writeCmdOutputTo("wg-show.txt", dirName, "sudo wg show"),
		)
	}

	if isContainerPresent(telegrafContainerName) {
		results = append(results, writeDockerLogsTo("telegraf-logs.txt", dirName, telegrafContainerName))
	}
	if isContainerPresent(dbMigrateContainerName) {
		results = append(results, writeDockerLogsTo("db-migrate-logs.txt", dirName, dbMigrateContainerName))
	}

	// tls-auth.json contains a private key
	if !opts.ExcludeSecrets {
		results = append(results, copyDockerFileTo("tls-auth.json", dirName, "eg-s:/etc/vpn-s/secrets.json"))
	}

	errors := packErrors(results...)

	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "%d subreport(s) failed.\nErrors will be also written into %s in the zip archive.\n\n", len(errors), failedReportFilename)
		writeErrorsTo(failedReportFilename, dirName, errors)
	} else {
		fmt.Println("All subreports were successfully created...")
	}

	// resolve zip file name
	simpleZipFileName := reportName + ".zip"
	zipFileName := filepath.Join(opts.OutputDirectory, simpleZipFileName)
	if opts.OutputDirectory == "" {
		zipFileName, err = filepath.Abs(simpleZipFileName)
		if err != nil {
			return fmt.Errorf("can't find out absolute path for output zip file: %w", err)
		}
	}

	// combine report files into one zip file
	fmt.Println("Creating report zip file... ")
	if err := createZipFile(zipFileName, dirName); err != nil {
		return fmt.Errorf("can't create zip file(%s): %w", zipFileName, err)
	}
	fmt.Printf("Done.\nReport file: %s\n", zipFileName)

	return nil
}

func isContainerPresent(containerName string) bool {
	_, _, err := runCmd("docker inspect --type=container " + containerName)
	return err == nil
}

func runCmd(cmdStr string) (stdout string, stderr string, err error) {
	cmd := exec.Command("bash", "-c", cmdStr)

	var outb bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err = cmd.Run()

	return outb.String(), errb.String(), err
}

func writeCmdOutputTo(fileName string, dirName string, cmdStr string) (err error) {
	stdout, stderr, err := runCmd(cmdStr)
	if err != nil {
		return fmt.Errorf("%s failed: %s", cmdStr, stderr)
	}

	path := filepath.Join(dirName, fileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("can't open file %s due to: %w", path, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("can't close file %s due to: %w", path, closeErr))
		}
	}()

	_, err = fmt.Fprintf(f, "Stdout:\n%s\n\nStderr:\n%s", stdout, stderr)
	if err != nil {
		return fmt.Errorf("write to file %s failed due to: %w", path, err)
	}
	return nil
}

// writeDockerLogsTo executes docker logs on the given container and saves the output to the specified file
func writeDockerLogsTo(fileName string, dirName string, containerName string) (err error) {
	dockerCmd := "docker logs " + containerName

	stdout, stderr, err := runCmd(dockerCmd)

	if err != nil {
		return fmt.Errorf("%s failed: %s", dockerCmd, stderr)
	}

	// open file (and close it in the end)
	path := filepath.Join(dirName, fileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("can't open file %s due to: %w", path, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("can't close file %s due to: %w", path, closeErr))
		}
	}()

	_, err = fmt.Fprintf(f, "Stdout:\n%s\n\nStderr:\n%s", stdout, stderr)
	if err != nil {
		return fmt.Errorf("write to file %s failed due to: %w", path, err)
	}

	return nil
}

// dumpDatabaseTo dumps the contents of a postgres database running on the given container
func dumpDatabaseTo(fileName string, dirName string, containerName string, dbName string, userName string, excludeSecrets bool) error {
	tableNames := []string{
		"ldap_templates",
		"users_to_ldaps",
		"servers",
		"wg_iface_addresses",
		"wg_iface_dns",
		"tags",
		"mfa_certs",
	}

	type secretTable struct {
		Name    string
		Columns string
	}

	// names of tables that contain secrets (passwords, private keys) with lists of their columns excluding columns with these secrets
	secretTables := []secretTable{
		{"users", "id, username, is_admin, auth_service, mfa_auth, ntf, updated_at"},
		{"ldaps", "id, priority, host, port, base_dn, bind_dn, user_list_filter, username_attribute, uid_attribute, use_tls, fqdn, ca_cert, template_id"},
		{"ldap_template_servers", "id, ldap_template_id, server_id, allowed_ips"},
		{"server_wg_configs", "id, server_id, interface_name, public_key, listen_port, mtu, persistent_keepalive"},
		{"adjacencies", "id, server_id, device_id, server_side_allowed_ips, client_side_allowed_ips"},
		{"root_ca", "id, cert"},
		{"totp", "id, user_id"},
	}

	// empty string (no -t options) means that all tables will be dumped
	includedTables := ""
	if excludeSecrets {
		for _, tableName := range tableNames {
			includedTables += " -t " + tableName
		}
	}

	dbDumpPath := dirName + "/" + fileName
	dockerCmd := fmt.Sprintf("docker exec %s pg_dump%s --data-only --username=%s %s > %s", containerName, includedTables, userName, dbName, dbDumpPath)
	_, stderr, err := runCmd(dockerCmd)

	if err != nil {
		return fmt.Errorf("%s failed: %s", dockerCmd, stderr)
	}

	// dumping tables with secrets is only neccessary if we haven't already dumped every table
	if excludeSecrets {
		for _, table := range secretTables {
			dumpPath := fmt.Sprintf("%s/database_%s.csv", dirName, table.Name)
			query := fmt.Sprintf("SELECT %s FROM %s", table.Columns, table.Name)
			dockerCmd = fmt.Sprintf("docker exec %s psql -d %s --username=%s --command=\"copy(%s) to stdout csv header\" > %s", containerName, dbName, userName, query, dumpPath)
			_, stderr, err = runCmd(dockerCmd)

			if err != nil {
				return fmt.Errorf("%s failed: %s", dockerCmd, stderr)
			}
		}
	}

	return nil
}

func copyDockerFileTo(destFileName string, destDirName string, sourceFilePath string) error {
	dockerCmd := "docker cp " + sourceFilePath + " " + destDirName + "/" + destFileName
	_, stderr, err := runCmd(dockerCmd)

	if err != nil {
		return fmt.Errorf("%s failed: %s", dockerCmd, stderr)
	}

	return nil
}

// createZipFile compresses content of directory dirName into a single zip archive file named filename.
// Both arguments should be absolut path file/directory names. The directory content excludes subdirectories.
func createZipFile(zipFileName string, dirName string) (err error) {
	// create zip writer
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return fmt.Errorf("can't create empty zip file(%v) due to: %v", zipFileName, err)
	}
	defer func() {
		if closeErr := zipFile.Close(); closeErr != nil {
			err = fmt.Errorf("can't close zip file %v due to: %v", zipFileName, closeErr)
		}
	}()
	zipWriter := zip.NewWriter(zipFile)
	defer func() {
		if closeErr := zipWriter.Close(); closeErr != nil {
			err = fmt.Errorf("can't close zip file writer for zip file %v due to: %v", zipFileName, closeErr)
		}
	}()

	// Add files to zip
	dirItems, err := os.ReadDir(dirName)
	if err != nil {
		return fmt.Errorf("can't read report directory(%v) due to: %v", dirName, err)
	}
	for _, dirItem := range dirItems {
		if !dirItem.IsDir() {
			if err = addFileToZip(zipWriter, filepath.Join(dirName, dirItem.Name())); err != nil {
				return fmt.Errorf("can't add file dirItem.Name() to report zip file due to: %v", err)
			}
		}
	}
	return nil
}

// addFileToZip adds file to zip file by using zip.Writer. The file name should be a absolute path.
func addFileToZip(zipWriter *zip.Writer, filename string) error {
	// open file for addition
	fileToZip, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("can't open file %v due to: %v", filename, err)
	}
	defer func() {
		if closeErr := fileToZip.Close(); closeErr != nil {
			err = fmt.Errorf("can't close zip file %v opened "+
				"for file appending due to: %v", filename, closeErr)
		}
	}()

	// get information from file for addition
	info, err := fileToZip.Stat()
	if err != nil {
		return fmt.Errorf("can't get information about file (%v) "+
			"that should be added to zip file due to: %v", filename, err)
	}

	// add file to zip file
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("can't create zip file info header for file %v due to: %v", filename, err)
	}
	header.Method = zip.Deflate // enables compression
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("can't create zip header for file %v due to: %v", filename, err)
	}
	_, err = io.Copy(writer, fileToZip)
	if err != nil {
		return fmt.Errorf("can't copy content of file %v to zip file due to: %v", filename, err)
	}
	return nil
}

func packErrors(errors ...error) []error {
	var errs []error
	for _, err := range errors {
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func writeErrorsTo(fileName string, dirName string, errors []error) {
	// open file (and close it in the end)
	path := filepath.Join(dirName, fileName)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't open file %s due to: %v\n", path, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "can't close file %s due to: %v\n", path, closeErr)
		}
	}()

	for _, subErr := range errors {
		if subErr != nil {
			fmt.Println(subErr)
			_, err = f.WriteString(subErr.Error() + "\n")
			if err != nil {
				fmt.Fprintf(os.Stderr, "write to file %s failed due to: %v\n", path, err)
			}
		}
	}
}
