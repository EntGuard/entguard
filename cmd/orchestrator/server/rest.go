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

package server

import (
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/rest"
)

func RunREST(s *rest.Server, cfg *config.RESTServerConfig, l *log.Logger, e chan error) {
	logger := l.WithField("reportCaller", "REST API")
	logger.Info("Starting REST...")
	http.HandleFunc("/", s.ServeHTTP)

	addr := fmt.Sprintf(":%s", cfg.Port)
	logger.
		WithField("port", cfg.Port).
		Info("Listening for Management UI")

	err := http.ListenAndServeTLS(addr, cfg.CertFile, cfg.KeyFile, nil)
	if err != nil {
		e <- err
		return
	}
}
