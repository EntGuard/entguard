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

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/cmd/vpn_server/vpns"
	"github.com/entguard/entguard/pkg/secret"
	config "github.com/entguard/entguard/pkg/server-config"
	"github.com/entguard/entguard/pkg/tls"
)

const logo = `
__      __           _____ 
\ \    / /          / ____|
 \ \  / / __  _ __ | (___  
  \ \/ / '_ \| '_ \ \___ \ 
   \  /| |_) | | | |____) |
    \/ | .__/|_| |_|_____/ 
       | |                 
       |_|              `

const (
	appName                = "VpnS"
	defaultConfigFilePath  = "/etc/vpn-s/config.json"
	defaultSecretsFilePath = "/etc/vpn-s/secrets.json"
)

func main() {
	fmt.Fprintln(os.Stderr, logo)

	cfgFile := flag.String("config", defaultConfigFilePath, "path to a json file with config")
	scrtFile := flag.String("secrets", defaultSecretsFilePath, "path to a json file with secrets")
	enableTelemetryRequest := flag.Bool("enable-telemetry", false, "request to enable telemetry, it will be enabled only if it is also an enabled feature in EG-O")
	flag.Parse()

	if err := runVpnS(*cfgFile, *scrtFile, *enableTelemetryRequest); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func runVpnS(cfgFile, scrtFile string, enableTelemetryRequest bool) error {
	l := log.New()
	l.SetFormatter(&log.TextFormatter{TimestampFormat: time.DateTime})
	l.SetLevel(log.DebugLevel)
	logger := l.WithField("reportCaller", appName)

	cfg, err := config.NewConfigFromFile(cfgFile, logger)
	if err != nil {
		return fmt.Errorf("failed to read config from file %q: %v", cfgFile, err)
	}

	logLevel, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %q", cfg.LogLevel)
	}
	logger.WithField("newLogLevel", logLevel).Info("Setting log level")
	logger.Logger.SetLevel(logLevel)

	scrt, err := secret.NewSecretFromFile(scrtFile)
	if err != nil {
		return fmt.Errorf("failed to read secrets from file %q: %v", scrtFile, err)
	}

	tlsCfg, err := tls.PrepareTLS(scrt, cfg)
	if err != nil {
		return fmt.Errorf("could not create TLS configuration: %v", err)
	}

	vpnS := vpns.NewVpnS(cfg, tlsCfg, enableTelemetryRequest, logger)

	err = vpnS.Connect(true)
	go vpnS.ReconnectOnError()
	defer func() {
		logger.Info("Shutting down.")
		vpnS.Disconnect(true)
	}()
	if err != nil {
		return err
	}

	// wait for SIGINT or SIGTERM
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	signal.Notify(ch, os.Interrupt)
	<-ch

	return nil
}
