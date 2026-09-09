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
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// ComposeProject runs docker compose commands against an already-running project.
type ComposeProject struct {
	file string
	name string
}

// NewComposeProject returns a handle on the project defined by file, which must
// be absolute or relative to the test's working directory.
func NewComposeProject(name, file string) *ComposeProject {
	return &ComposeProject{file: file, name: name}
}

// NewRESTFuzzComposeProject returns the project backing the REST API fuzz test.
func NewRESTFuzzComposeProject() (*ComposeProject, error) {
	file := os.Getenv(restFuzzComposeFileEnvVar)
	if file == "" {
		return nil, fmt.Errorf("%s is not set: run this test through `make test-fuzz-rest`", restFuzzComposeFileEnvVar)
	}
	return NewComposeProject(restFuzzComposeProject, file), nil
}

// ComposeEvent is one line of `docker compose events --json`.
type ComposeEvent struct {
	Time       time.Time         `json:"time"`
	Type       string            `json:"type"`
	Service    string            `json:"service"`
	ID         string            `json:"id"`
	Action     string            `json:"action"`
	Attributes map[string]string `json:"attributes"`
}

func (p *ComposeProject) command(ctx context.Context, args ...string) *exec.Cmd {
	base := []string{"compose", "--file", p.file, "--project-name", p.name}
	return exec.CommandContext(ctx, "docker", append(base, args...)...)
}

// Restart restarts the named services and their dependencies, allowing timeout
// for each to stop before it is killed.
func (p *ComposeProject) Restart(ctx context.Context, timeout time.Duration, services ...string) error {
	args := append([]string{"restart", "--timeout", strconv.Itoa(int(timeout.Seconds()))}, services...)
	cmd := p.command(ctx, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose restart %v: %w: %s", services, err, out)
	}
	return nil
}

// DumpLogs writes the project's logs to file, service-prefixed and timestamped.
func (p *ComposeProject) DumpLogs(ctx context.Context, file string) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	cmd := p.command(ctx, "logs", "--no-color", "--timestamps")
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose logs: %w", err)
	}
	return nil
}

// Events streams container events for the named services until ctx is cancelled,
// the CLI exits, or consumer returns an error, which is returned unchanged.
func (p *ComposeProject) Events(ctx context.Context, services []string, consumer func(ComposeEvent) error) error {
	// Cancelling stops the CLI once the consumer is done.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := p.command(ctx, append([]string{"events", "--json"}, services...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("docker compose events: %w", err)
	}

	var consumerErr error
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event ComposeEvent
		if err := json.Unmarshal(line, &event); err != nil {
			// The stream is only a watchdog; skip lines we cannot parse.
			continue
		}
		if err := consumer(event); err != nil {
			consumerErr = err
			break
		}
	}

	cancel()
	_ = cmd.Wait()

	if consumerErr != nil {
		return consumerErr
	}
	// Ignore scan errors caused by our own teardown.
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}
