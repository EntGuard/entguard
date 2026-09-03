/*
 * Copyright 2024 PANTHEON.tech s.r.o.
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

package asroot

import (
	"errors"
	"os"
	"os/exec"
)

var (
	ErrInvalidCmd = errors.New("invalid command for setting EUID to 0")
)

const UserEnv = "USER"
const SudoUserEnv = "SUDO_USER"
const DoasUserEnv = "DOAS_USER"

type EUIDZeroCmd int

const (
	None EUIDZeroCmd = iota
	Sudo
	Doas
)

func (c EUIDZeroCmd) String() string {
	switch c {
	case None:
		return ""
	case Sudo:
		return "sudo"
	case Doas:
		return "doas"
	default:
		return ""
	}
}

func (c EUIDZeroCmd) UsedByUser() (string, error) {
	switch c {
	case None:
		return os.Getenv(UserEnv), nil
	case Sudo:
		return os.Getenv(SudoUserEnv), nil
	case Doas:
		return os.Getenv(DoasUserEnv), nil
	default:
		return "", ErrInvalidCmd
	}
}

func (c EUIDZeroCmd) Path() (string, error) {
	switch c {
	case None, Sudo, Doas:
		return exec.LookPath(c.String())
	default:
		return "", ErrInvalidCmd
	}
}

func (c EUIDZeroCmd) Exec(name string, arg ...string) (*exec.Cmd, error) {
	switch c {
	case None:
		return exec.Command(name, arg...), nil
	case Sudo, Doas:
		return exec.Command(c.String(), append([]string{name}, arg...)...), nil
	default:
		return nil, ErrInvalidCmd
	}
}

func AvailableCmds() ([]EUIDZeroCmd, error) {
	var allCmds = []EUIDZeroCmd{Sudo, Doas}
	var cmds []EUIDZeroCmd
	var err error
	for _, c := range allCmds {
		if _, e := c.Path(); e != nil {
			err = errors.Join(err, e)
		}
		cmds = append(cmds, c)
	}
	if len(cmds) == 0 {
		return []EUIDZeroCmd{None}, err
	}
	return cmds, nil
}

func Currently() (bool, EUIDZeroCmd) {
	if os.Geteuid() != 0 {
		return false, None
	}
	if os.Getenv(SudoUserEnv) != "" {
		return true, Sudo
	}
	if os.Getenv(DoasUserEnv) != "" {
		return true, Doas
	}
	return true, None
}
