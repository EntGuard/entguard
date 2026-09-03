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

package logger

import (
	"time"

	log "github.com/sirupsen/logrus"
)

type colorHook struct{}

func (h *colorHook) Levels() []log.Level {
	return log.AllLevels
}

var (
	red    = "\033[31m"
	reset  = "\033[0m"
	yellow = "\033[33m"
)

func (h *colorHook) Fire(entry *log.Entry) error {
	switch entry.Level {
	case log.ErrorLevel, log.FatalLevel, log.PanicLevel:
		entry.Message = red + entry.Message + reset
	case log.WarnLevel:
		entry.Message = yellow + entry.Message + reset
	}
	return nil
}

func SetupLogger(l *log.Logger) {
	l.SetFormatter(&log.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		TimestampFormat: time.DateTime,
	})

	l.AddHook(&colorHook{})
}
