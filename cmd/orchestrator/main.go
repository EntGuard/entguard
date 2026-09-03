/*
 * Copyright 2021 PANTHEON.tech s.r.o.
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
	"context"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/cmd/orchestrator/server"
	"github.com/entguard/entguard/pkg/conc"
	"github.com/entguard/entguard/pkg/db"
	logsetup "github.com/entguard/entguard/pkg/logger"
	"github.com/entguard/entguard/service/buildinfo"
	"github.com/entguard/entguard/service/config"
	"github.com/entguard/entguard/service/rest"
	"github.com/entguard/entguard/service/rpc"
	"github.com/entguard/entguard/service/user"
	"github.com/entguard/entguard/service/vcm"
)

const buildInfoPrint = `
	The EntGuard Orchestrator %s
	----------------------------------
	Build Info:
		Git commit: %s
		Git branch: %s

`

func main() {
	fmt.Fprintf(os.Stderr, buildInfoPrint, buildinfo.Version(), buildinfo.GitCommit(), buildinfo.GitBranch())

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run() error {
	logger := log.New()
	logsetup.SetupLogger(logger)

	logger.Info("Reading configuration.")
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("could not read configuration: %v", err)
	}

	logger.SetLevel(cfg.LogLevel)
	ctx := context.Background()

	logger.Info("Connecting to database...")
	conn, err := db.Connect(ctx, logger, cfg.DB)
	if err != nil {
		return fmt.Errorf("could not create Postgres connection pool and connect to DB: %w", err)
	}
	logger.Info("Database connection established.")

	encryptionKey, err := config.ReadEncryptionKey(cfg.EncryptionKeyPath)
	if err != nil {
		return err
	}

	logger.Info("Setting up VPN configuration manager (VCM)...")
	vpnConfigManager, err := vcm.NewVCMPostgresBased(ctx, conn, logger, cfg.VCM.ReinitCert, encryptionKey)
	if err != nil {
		return fmt.Errorf("could not setup VPN configuration manager (VCM): %w", err)
	}
	logger.Info("VCM successfully created.")

	logger.Info("Setting up user manager...")
	userManager, err := user.NewPostgresManager(ctx, cfg.UserManager, vpnConfigManager, conn, logger, encryptionKey)
	if err != nil {
		return fmt.Errorf("could not setup user manager: %w", err)
	}
	logger.Info("User manager successfully created.")

	logger.Info("Enforcing DPU limit on startup...")
	if err := userManager.EnforceDPULimit(ctx); err != nil {
		logger.WithError(err).Error("Enforcing DPU limit on startup")
	}

	logger.Info("Setting up VPN server status store...")
	// vpnSStatusStore represents VPN server current status
	// in REST service based on gRPC communication status
	vpnSStatusStore := conc.NewMap[int, error]()
	logger.Info("Status store successfully created.")

	// hcStatusStore represents healthcheck current status
	// in REST service based on gRPC communication status
	// Note: atomic.Value (for current case of 1 healthcheck)
	// is bad solution as it remembers first error type and
	// it panic if it gets other error type later -> sticking
	// with suboptimal conc.NewMap
	hcStatusStore := rpc.NewHealthcheckStatusStore()
	logger.Info("Healthcheck status store successfully created.")

	logger.Info("Creating REST service...")
	restServer, err := rest.NewServer(
		cfg.REST, vpnConfigManager, userManager, vpnSStatusStore, hcStatusStore, logger,
	)
	if err != nil {
		return fmt.Errorf("could not create REST service: %v", err)
	}
	logger.Info("Service successfully created.")

	logger.Info("Creating gRPC service...")
	rpcServer, err := rpc.NewServer(
		ctx, cfg.RPC, vpnConfigManager, userManager, vpnSStatusStore, hcStatusStore, logger,
	)
	if err != nil {
		return fmt.Errorf("could not create RPC service: %v", err)
	}
	logger.Info("Service successfully created.")

	// TODO: maybe rework/check error handling and synchronization of these goroutines
	fails := make(chan error)

	go vpnConfigManager.HandlePeerUpdates(ctx, fails)
	go vpnConfigManager.HandleServerUpdates(ctx, fails)
	go vpnConfigManager.HandleSessionsUpdatesForHealthcheckService(ctx, fails)
	go server.RunREST(restServer, cfg.REST, logger, fails)
	go server.RunRPC(rpcServer, cfg.RPC, logger, fails)

	// Block until one of servers returns an error.
	err = <-fails

	return err
}
