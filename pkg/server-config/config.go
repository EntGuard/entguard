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

package serverconfig

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
)

// When updating these defaults, you may also want to update
// defaults for "egvpn config init" command in "cmd/egvpn/cmd/config.go".
const (
	// Do not use "localhost" instead of "127.0.0.1". There is a bug in golang
	// grpc library with DNS resolving. If you run the egserver in a VM with
	// default (NAT) networking, network adapter is up in guest OS and network
	// connectivity is down in host OS, resolving "localhost" takes 10 seconds.
	defaultGRPCAddress = "127.0.0.1:8081"
	// Must be at least 10 seconds plus some headroom to double guard against
	// the aforementioned bug.
	defaultGRPCTimeout       = 15 * time.Second
	defaultCaName            = "cnf-vpn-o.local"
	defaultReconnectAttempts = 10
	defaultReconnectPause    = 5 * time.Second
	defaultLogLevel          = "info"

	defaultTokensPerSecond = 20
	defaultBucketSize      = 500

	defaultHCGRPCAddress = "127.0.0.1:8083"
	defaultHCListenPort  = 8084

	defaultHCTokensPerSecond = 20  // 200 devices with 20 seconds healthcheck refresh = 10 tokens per second (multiply by 2 for buffer and irregular hc ping distribution)
	defaultHCBucketSize      = 500 // burst buffer for temporal high traffic
)

var (
	errEmptyCAcert = errors.New("provided CA certificate is empty")
)

type Config struct {
	UseVPP bool // use VPP as Wireguard backend (default is false and that means to use linux kernel wireguard backend)

	RPCAddr    string        `json:"grpcAddr"`
	RPCTimeout time.Duration `json:"grpcTimeout"`

	// CaName is used for Server Name Indication (SNI) in TLS configuration"
	CaName string `json:"caName"`
	CaCert []byte `json:"caCert"`

	ReconnectAttempts int           `json:"reconnectAttempts"`
	ReconnectPause    time.Duration `json:"reconnectPause"`

	LogLevel string `json:"logLevel"`

	// Rate limiting communication (GRPC)
	TokensPerSecond float64 `json:"grpcRatelimiterTokensPerSecond"`
	BucketSize      int     `json:"grpcRatelimiterBucketSize"`

	HCGRPCAddr   string
	HCListenPort int

	// HealthCheck Rate limiting GRPC communication (session updates from orchestrator and GRPC pings from clients)
	HCTokensPerSecond float64 `json:"hcGrpcRatelimiterTokensPerSecond"`
	HCBucketSize      int     `json:"hcGrpcRatelimiterBucketSize"`
}

// NewConfigFromFile reads extra data for config from the given path.
func NewConfigFromFile(path string, logger *log.Entry) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c Config
	err = json.Unmarshal(data, &c)
	if err != nil {
		return nil, err
	}

	if len(c.CaCert) == 0 {
		return nil, errEmptyCAcert
	}

	if c.RPCAddr == "" {
		c.RPCAddr = defaultGRPCAddress
		if logger != nil {
			logger.WithField("defaultGRPCAddress", defaultGRPCAddress).
				Info("RPC address is not specified, the default one will be used")
		}
	}
	if c.RPCTimeout == 0 {
		c.RPCTimeout = defaultGRPCTimeout
		if logger != nil {
			logger.WithField("defaultGRPCTimeout", defaultGRPCTimeout).
				Info("RPC timeout is not specified, the default timeout will be used")
		}
	}
	if c.CaName == "" {
		c.CaName = defaultCaName
		if logger != nil {
			logger.WithField("defaultCaName", defaultCaName).
				Info("Server name is not specified, the default name will be used")
		}
	}
	if c.ReconnectAttempts == 0 {
		c.ReconnectAttempts = defaultReconnectAttempts
		if logger != nil {
			logger.WithField("defaultReconnectAttempts", defaultReconnectAttempts).
				Info("Number of reconnect attempts is not specified, the default value will be used")
		}
	}
	if c.ReconnectPause == 0 {
		c.ReconnectPause = defaultReconnectPause
		if logger != nil {
			logger.WithField("defaultReconnectPause", defaultReconnectPause).
				Info("Pause between reconnects attempts is not specified, the default value will be used")
		}
	}
	if c.LogLevel == "" {
		c.LogLevel = defaultLogLevel
		if logger != nil {
			logger.WithField("defaultLogLevel", defaultLogLevel).
				Info("Log level is not specified, the default value will be used")
		}
	}
	if c.TokensPerSecond == 0.0 { // 0.0 means that rate limiter will stop all traffic and that is something that we don't want -> using 0.0 as uninitialized value
		c.TokensPerSecond = defaultTokensPerSecond
		if logger != nil {
			logger.WithField("defaultTokensPerSecond", defaultTokensPerSecond).
				Info("Server GRPC rate limiter's tokens per second setting is not specified, the default one will be used")
		}
	}
	if c.BucketSize == 0 {
		c.BucketSize = defaultBucketSize
		if logger != nil {
			logger.WithField("defaultBucketSize", defaultBucketSize).
				Info("Server GRPC rate limiter's bucket size setting is not specified, the default one will be used")
		}
	}
	if c.HCGRPCAddr == "" {
		c.HCGRPCAddr = defaultHCGRPCAddress
		if logger != nil {
			logger.WithField("defaultHCGRPCAddress", defaultHCGRPCAddress).
				Info("Healthcheck configuration RPC address is not specified, the default one will be used")
		}
	}
	if c.HCListenPort == 0 {
		c.HCListenPort = defaultHCListenPort
		if logger != nil {
			logger.WithField("defaultHCListenPort", defaultHCListenPort).
				Info("Healthcheck listen port is not specified, the default one will be used")
		}
	}
	if c.HCTokensPerSecond == 0.0 { // 0.0 means that rate limiter will stop all traffic and that is something that we don't want -> using 0.0 as uninitialized value
		c.HCTokensPerSecond = defaultHCTokensPerSecond
		if logger != nil {
			logger.WithField("defaultHCTokensPerSecond", defaultHCTokensPerSecond).
				Info("Healthcheck GRPC rate limiter's tokens per second setting is not specified, the default one will be used")
		}
	}
	if c.HCBucketSize == 0 {
		c.HCBucketSize = defaultHCBucketSize
		if logger != nil {
			logger.WithField("defaultHCBucketSize", defaultHCBucketSize).
				Info("Healthcheck GRPC rate limiter's bucket size setting is not specified, the default one will be used")
		}
	}

	return &c, nil
}
