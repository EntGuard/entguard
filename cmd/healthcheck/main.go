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

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	health "github.com/entguard/entguard/cmd/healthcheck/orchestrator"
	"github.com/entguard/entguard/cmd/healthcheck/vpn_client"
	"github.com/entguard/entguard/pkg/secret"
	config "github.com/entguard/entguard/pkg/server-config"
	"github.com/entguard/entguard/pkg/tls"
)

const (
	appName                = "HealthCheck"
	defaultConfigFilePath  = "/etc/vpn-s/config.json"
	defaultSecretsFilePath = "/etc/vpn-s/secrets.json"
)

func main() {
	cfgFile := flag.String("config", defaultConfigFilePath, "path to a json file with config")
	scrtFile := flag.String("secrets", defaultSecretsFilePath, "path to a json file with secrets")
	flag.Parse()

	if err := runHealthcheck(*cfgFile, *scrtFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func runHealthcheck(cfgFile, scrtFile string) error {
	l := log.New()
	l.SetFormatter(&log.TextFormatter{TimestampFormat: time.DateTime})
	l.SetLevel(log.DebugLevel)
	logger := l.WithField("reportCaller", appName)

	cfg, err := config.NewConfigFromFile(cfgFile, logger)
	if err != nil {
		return fmt.Errorf("read config from file %q: %v", cfgFile, err)
	}

	logLevel, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %q", cfg.LogLevel)
	}
	logger.WithField("newLogLevel", logLevel).Info("Setting log level")
	logger.Logger.SetLevel(logLevel)

	scrt, err := secret.NewSecretFromFile(scrtFile)
	if err != nil {
		return fmt.Errorf("read secrets from file %q: %v", scrtFile, err)
	}

	tlsCfg, err := tls.PrepareTLS(scrt, cfg)
	if err != nil {
		return fmt.Errorf("create TLS configuration: %v", err)
	}

	hc := health.NewHealthcheck(cfg, tlsCfg, logger)

	// starts gRPC client to connect with orchestrator
	err = hc.Connect()
	go hc.ReconnectOnError()
	defer func() {
		logger.Info("Shutting down.")
		hc.Disconnect()
	}()
	if err != nil {
		return err
	}

	fails := make(chan error)

	// starts gRPC server to listen for healthcheck pings from vpn client
	go vpn_client.RunGRPCServerForVPNClient(logger, hc, cfg, fails)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGTERM)
	signal.Notify(interrupt, os.Interrupt)

	select {
	case err := <-fails:
		return err
	case <-interrupt:
	}

	return nil
}
