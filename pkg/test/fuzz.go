/*
 * Copyright 2026 PANTHEON.tech s.r.o.
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

package test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ffuf/ffuf/v2/pkg/ffuf"
	"github.com/ffuf/ffuf/v2/pkg/filter"
	"github.com/ffuf/ffuf/v2/pkg/input"
	"github.com/ffuf/ffuf/v2/pkg/output"
	"github.com/ffuf/ffuf/v2/pkg/runner"
	"github.com/stretchr/testify/require"
	"gitlab.com/michenriksen/jdam/pkg/jdam"
	"gitlab.com/michenriksen/jdam/pkg/jdam/mutation"
	"golang.org/x/sync/errgroup"

	"github.com/entguard/entguard/pkg/httpconst"
	"github.com/entguard/entguard/service/api/v1"
	"github.com/entguard/entguard/service/user"
)

type FuzzFilepaths struct {
	WordlistFile   string
	FFUFOutputFile string
	FFUFLogFile    string
	ComposeLogFile string
}

func SkipOrSetupRESTFuzzTest(t *testing.T, ctx context.Context, q user.Querier, compose *ComposeProject, testDir string) (*api.JWT, *FuzzFilepaths) {
	if !IsRESTFuzzTest() {
		t.Skipf("skipping REST API fuzz test: to run this test set %s envvar to 1", restFuzzEnvVar)
	}

	paths := &FuzzFilepaths{
		WordlistFile:   filepath.Join(testDir, t.Name()+"-wordlist.txt"),
		FFUFOutputFile: filepath.Join(testDir, t.Name()+"-ffuf-output.txt"),
		FFUFLogFile:    filepath.Join(testDir, t.Name()+"-ffuf.log"),
		ComposeLogFile: filepath.Join(testDir, t.Name()+"-compose.log"),
	}

	err := compose.Restart(ctx, 20*time.Second, restFuzzOrchestratorService)
	require.NoError(t, err)

	// give some time for orchestrator restart
	// TODO: figure out a smarter way than hard sleep
	time.Sleep(3 * time.Second)

	err = TruncateAllTables(ctx, q.GetDbConnection(ctx))
	require.NoError(t, err)
	_ = InsertSampleUser(t, ctx, q, TestUserName)

	go checkDockerComposeServices(t, ctx, compose)

	jwt, err := FetchJWT(TestUserName, TestUserPassword, TestOrchestratorURL)
	require.NoError(t, err)
	return jwt, paths
}

func RunFFUFJob(
	ctx context.Context,
	method string,
	url string,
	wordlists []string,
	jwt *api.JWT,
	logFile string,
	outputFile string,
) error {
	opts := ffuf.NewConfigOptions()
	opts.Input.Wordlists = wordlists
	opts.HTTP.Headers = []string{
		fmt.Sprintf("%s: %s", httpconst.ContentType, httpconst.ApplicationJSON),
		fmt.Sprintf("%s: Bearer %s", httpconst.Authorization, jwt.Token),
	}
	opts.General.Noninteractive = true
	opts.HTTP.Method = method
	opts.HTTP.URL = url
	if method != http.MethodGet {
		opts.HTTP.Data = "FUZZ"
	}
	opts.Output.OutputFormat = "json"
	opts.Output.OutputFile = outputFile
	opts.Output.DebugLog = logFile
	ffufCtx, cancel := context.WithCancel(ctx)
	config, err := ffuf.ConfigFromOptions(opts, ffufCtx, cancel)
	if err != nil {
		return err
	}
	config.MatcherManager = filter.NewMatcherManager()
	job := ffuf.NewJob(config)
	job.Runner = runner.NewSimpleRunner(config, false)
	var errs ffuf.Multierror
	job.Input, errs = input.NewInputProvider(config)
	if errs.ErrorOrNil() != nil {
		return errs.ErrorOrNil()
	}
	job.Output = output.NewStdoutput(config)
	job.Start()
	return nil
}

func DumpComposeLogs(ctx context.Context, compose *ComposeProject, file string) error {
	return compose.DumpLogs(ctx, file)
}

func checkDockerComposeServices(t *testing.T, ctx context.Context, compose *ComposeProject) {
	errServiceDown := errors.New("docker compose service is down")
	err := compose.Events(ctx, []string{restFuzzOrchestratorService, restFuzzDBService},
		func(event ComposeEvent) error {
			switch event.Action {
			case "destroy", "kill", "stop", "die":
				return fmt.Errorf("%w: detected event %s for service %s", errServiceDown, event.Action, event.Service)
			}
			return nil
		})
	if errors.Is(err, errServiceDown) {
		require.NoError(t, err)
	}
}

func valueToMap(v any) (map[string]any, error) {
	subjectJSON, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	err = json.Unmarshal(subjectJSON, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func generateFuzzedValues(fuzzer *jdam.Fuzzer, val any, count int, ch chan<- []byte) error {
	defer close(ch)
	valMap, err := valueToMap(val)
	if err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		fuzzed := fuzzer.Fuzz(valMap)
		fuzzedJSON, err := json.Marshal(fuzzed)
		if err != nil {
			return err
		}
		ch <- fuzzedJSON
	}
	return nil
}

func WriteJSONWordlist[T any](w io.Writer, values []T, count int) error {
	errg, ctx := errgroup.WithContext(context.Background())

	bw := bufio.NewWriter(w)
	defer func() { _ = bw.Flush() }()

	recv := make(chan []byte)
	for _, val := range values {
		val := val
		ch := make(chan []byte)
		fuzzer := jdam.New(mutation.Mutators)
		errg.Go(func() error {
			return generateFuzzedValues(fuzzer, val, count, ch)
		})
		errg.Go(func() error {
			for fuzzed := range ch {
				select {
				case recv <- fuzzed:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}
	errCh := make(chan error)
	go func(errCh chan<- error) {
		defer close(errCh)
		newl := '\n'
		for val := range recv {
			_, err := bw.Write(append(val, byte(newl)))
			if err != nil {
				errCh <- err
				return
			}
		}
	}(errCh)
	if err := errg.Wait(); err != nil {
		close(recv)
		return err
	}
	close(recv)
	return <-errCh
}

func WriteJSONWordlistToFile[T any](path string, values []T, count int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	return WriteJSONWordlist(f, values, count)
}
