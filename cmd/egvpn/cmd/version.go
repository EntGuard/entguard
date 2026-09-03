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

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Version shows CLI utility version, and image version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVersion()
		},
	}
}

func runVersion() (resultErr error) {
	fmt.Printf(`EGVPN %s
	Build Info:
		Git Commit: %s
		Git Branch: %s
`, buildinfo.Version(), buildinfo.GitCommit(), buildinfo.GitBranch())

	errNoImageFound := "failed to find EntGuard docker image %s"

	sCommand := fmt.Sprintf("docker images --filter=reference='%s'", buildinfo.EgServerImageName())
	sInfo, err := exec.Command("/bin/bash", "-c", sCommand).CombinedOutput()
	if err != nil {
		fmt.Println(string(sInfo))
		resultErr = errors.Join(resultErr, fmt.Errorf(errNoImageFound, buildinfo.EgServerImageName()))
	} else {
		fmt.Printf("\nEntGuard Server Image Info: \n%s\n", string(sInfo))
	}

	hcCommand := fmt.Sprintf("docker images --filter=reference='%s'", buildinfo.EgHealthcheckImageName())
	hcInfo, err := exec.Command("/bin/bash", "-c", hcCommand).CombinedOutput()
	if err != nil {
		fmt.Println(string(hcInfo))
		resultErr = errors.Join(resultErr, fmt.Errorf(errNoImageFound, buildinfo.EgHealthcheckImageName()))
	} else {
		fmt.Printf("\nEntGuard Healthcheck Image Info: \n%s\n", string(hcInfo))
	}

	return resultErr
}
