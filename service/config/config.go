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

package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"time"

	"dario.cat/mergo"
	log "github.com/sirupsen/logrus"

	"github.com/entguard/entguard/pkg/crypto/aesgcm"
	"github.com/entguard/entguard/pkg/mfa"
)

// Environment variables names.
const (
	serviceEncryptionKeyPathEnv = "VPN_ORCHESTRATOR_ENCRYPTION_KEY_FILE"

	serviceRESTPortEnv = "VPN_ORCHESTRATOR_REST_PORT"

	serviceRPCPortVpnSEnv        = "VPN_ORCHESTRATOR_RPC_PORT_VPN_S"
	serviceRPCPortVpnCEnv        = "VPN_ORCHESTRATOR_RPC_PORT_VPN_C"
	serviceRPCPortHealthcheckEnv = "VPN_ORCHESTRATOR_RPC_PORT_HEALTH_CHECK"
	healthcheckRPCPortVpnCEnv    = "VPN_HEALTH_CHECK_RPC_PORT_VPN_C"
	serviceRPCCertFileEnv        = "VPN_ORCHESTRATOR_RPC_CERT_FILE"
	serviceRPCKeyFileEnv         = "VPN_ORCHESTRATOR_RPC_KEY_FILE"

	authTypeEnv       = "VPN_ORCHESTRATOR_AUTH_TYPE"
	vcmStorageTypeEnv = "VPN_ORCHESTRATOR_VCM_STORAGE"

	mfaTOTPEnv        = "VPN_ORCHESTRATOR_2FA_TOTP"
	mfaCertificateEnv = "VPN_ORCHESTRATOR_2FA_CERTIFICATE"

	reinitCertProviderEnv = "VPN_ORCHESTRATOR_RPC_REINIT_CERT_PROVIDER"

	// These vars are used for rate limiter that controls how frequently auth requests are allowed to happen.
	// It implements a "token bucket" of size b, initially full and refilled at rate r tokens per second.
	// For more info: https://en.wikipedia.org/wiki/Token_bucket
	grpcBucketSizeEnv      = "VPN_ORCHESTRATOR_RPC_RL_SIZE"
	grpcTokensPerSecondEnv = "VPN_ORCHESTRATOR_RPC_RL_RATE"
	restBucketSizeEnv      = "VPN_ORCHESTRATOR_REST_RL_SIZE"
	restTokensPerSecondEnv = "VPN_ORCHESTRATOR_REST_RL_RATE"

	passwordMinLengthEnv = "VPN_ORCHESTRATOR_PWD_MIN_LEN"
	passwordLowerEnv     = "VPN_ORCHESTRATOR_PWD_LOWER"
	passwordUpperEnv     = "VPN_ORCHESTRATOR_PWD_UPPER"
	passwordDigitEnv     = "VPN_ORCHESTRATOR_PWD_DIGIT"
	passwordSymbolEnv    = "VPN_ORCHESTRATOR_PWD_SYMBOL"

	logLevelEnv = "VPN_ORCHESTRATOR_LOG_LEVEL"

	ldapSyncIntervalEnv = "VPN_ORCHESTRATOR_LDAP_SYNC_INTERVAL"

	authTokenExpirationEnv = "VPN_ORCHESTRATOR_AUTH_TOKEN_EXPIRATION"

	sessionExpirationEnv       = "VPN_ORCHESTRATOR_SESSION_EXPIRATION"
	sessionExpCheckIntervalEnv = "VPN_ORCHESTRATOR_SESSION_EXP_CHECK_INTERVAL"

	// Postgres specific environment variables
	pgHostEnv     = "PGHOST"
	pgPortEnv     = "PGPORT"
	pgDatabaseEnv = "PGDATABASE"
	pgUserEnv     = "PGUSER"
	pgPasswordEnv = "PGPASSWORD"
)

// Default values.
const (
	serviceRESTPortDefault           = "8080"
	serviceRPCPortDefaultVpnS        = "8081"
	serviceRPCPortDefaultVpnC        = "8082"
	serviceRPCPortDefaultHealthcheck = "8083"
	healthcheckRPCPortDefaultVpnC    = "8084"
	authTypeDefault                  = "jwt"
	vcmStorageTypeDefault            = "postgres"

	mfaTOTPDefault        = "false"
	mfaCertificateDefault = "false"

	grpcBucketSizeDefault      = "500"
	grpcTokensPerSecondDefault = "10"
	restBucketSizeDefault      = "500"
	restTokensPerSecondDefault = "10"

	passwordMinDefault    = "15"
	passwordLowerDefault  = "false"
	passwordUpperDefault  = "false"
	passwordDigitDefault  = "false"
	passwordSymbolDefault = "false"

	reinitCertProviderDefault = "false"

	zeroDuration = 0 * time.Second

	ldapSyncIntervalDefault = "1h"

	authTokenExpirationDefault = "30m"

	sessionExpirationDefault       = "168h"
	sessionExpCheckIntervalDefault = "1h"
)

type RESTServerConfig struct {
	AuthType        string
	Port            string
	CertFile        string
	KeyFile         string
	Features        FeaturesData
	TokenExpiration time.Duration

	TokensPerSecond float64
	BucketSize      int
}

type RPCServerConfig struct {
	PortVpnS        string
	PortVpnC        string
	PortHealthcheck string
	CertFile        string
	KeyFile         string

	TokensPerSecond float64
	BucketSize      int

	Features FeaturesData
}

type PasswordConfig struct {
	MinLength int
	Uppercase bool
	Lowercase bool
	Symbol    bool
	Digit     bool
}

type VCMConfig struct {
	ReinitCert bool
	Features   FeaturesData
}

// UserManagementConfig  is used to configure userManager.
type UserManagementConfig struct {
	LdapSyncInterval        time.Duration
	Features                FeaturesData
	PwdCfg                  *PasswordConfig
	MFAEnabledTypes         []mfa.AuthenticatorType
	HealthcheckRPCPortVpnC  string
	SessionExpiration       time.Duration
	SessionExpCheckInterval time.Duration
}

type PostgresConfig struct {
	Host         string
	Port         uint16
	Database     string
	User         string
	Password     string
	ConnPoolSize int
}

// DefaultPostgresConfig returns database Config filled with default values.
func DefaultPostgresConfig() *PostgresConfig {
	const minPoolSize = 4

	var poolSize = minPoolSize
	if runtime.NumCPU() > minPoolSize {
		poolSize = runtime.NumCPU()
	}
	return &PostgresConfig{
		Host:         "localhost",
		Port:         5432,
		Database:     "entguard",
		User:         "postgres",
		Password:     "postgres",
		ConnPoolSize: poolSize,
	}
}

func PostgresConfigFromEnv() (*PostgresConfig, error) {
	cfg := DefaultPostgresConfig()
	envCfg := &PostgresConfig{
		Host:     os.Getenv(pgHostEnv),
		Database: os.Getenv(pgDatabaseEnv),
		User:     os.Getenv(pgUserEnv),
		Password: os.Getenv(pgPasswordEnv),
	}
	if p := os.Getenv(pgPortEnv); p != "" {
		port, err := strconv.Atoi(os.Getenv("PGPORT"))
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s environment variable: %w", pgPortEnv, err)
		}
		if port < 0 || port > math.MaxUint16 {
			return nil, fmt.Errorf("port %d is outside of allowed range (0-%d)", port, math.MaxUint16)
		}
		envCfg.Port = uint16(port)
	}
	if err := mergo.Merge(cfg, envCfg, mergo.WithOverride); err != nil {
		return nil, fmt.Errorf("failed to create postgres config from environment variables: %w", err)
	}
	return cfg, nil
}

func (c *PostgresConfig) ToURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?pool_max_conns=%d",
		c.User, c.Password, c.Host, c.Port, c.Database, c.ConnPoolSize)
}

// Config holds all required configuration for the service.
type Config struct {
	REST              *RESTServerConfig
	RPC               *RPCServerConfig
	VCM               *VCMConfig
	UserManager       *UserManagementConfig
	DB                *PostgresConfig
	LogLevel          log.Level
	Features          FeaturesData
	EncryptionKeyPath string
}

// FromEnv initialize configuration using environment variables.
func FromEnv() (*Config, error) {
	restPort := envOrDefault(serviceRESTPortEnv, serviceRESTPortDefault)
	rpcPortVpnS := envOrDefault(serviceRPCPortVpnSEnv, serviceRPCPortDefaultVpnS)
	rpcPortVpnC := envOrDefault(serviceRPCPortVpnCEnv, serviceRPCPortDefaultVpnC)
	rpcPortHealthcheck := envOrDefault(serviceRPCPortHealthcheckEnv, serviceRPCPortDefaultHealthcheck)
	healthcheckRPCPortVpnC := envOrDefault(healthcheckRPCPortVpnCEnv, healthcheckRPCPortDefaultVpnC)
	rpcCert := envOrDefault(serviceRPCCertFileEnv, "")
	rpcKey := envOrDefault(serviceRPCKeyFileEnv, "")
	authType := envOrDefault(authTypeEnv, authTypeDefault)

	encryptionKeyPath := envOrDefault(serviceEncryptionKeyPathEnv, "")

	var logLevel log.Level
	switch os.Getenv(logLevelEnv) {
	case "debug":
		logLevel = log.DebugLevel
	case "info":
		logLevel = log.InfoLevel
	case "warn", "warning":
		logLevel = log.WarnLevel
	default:
		logLevel = log.WarnLevel
	}

	grpcBucketSize, err := strconv.Atoi(envOrDefault(grpcBucketSizeEnv, grpcBucketSizeDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse grpc rate limiting's bucket size: %v", err)
	}
	grpcTps, err := strconv.ParseFloat(envOrDefault(grpcTokensPerSecondEnv, grpcTokensPerSecondDefault), 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse grpc rate limiting's tokens per second: %v", err)
	}
	restBucketSize, err := strconv.Atoi(envOrDefault(restBucketSizeEnv, restBucketSizeDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse REST rate limiting's bucket size: %v", err)
	}
	restTps, err := strconv.ParseFloat(envOrDefault(restTokensPerSecondEnv, restTokensPerSecondDefault), 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse REST rate limiting's tokens per second: %v", err)
	}

	pwdMinLen, err := strconv.Atoi(envOrDefault(passwordMinLengthEnv, passwordMinDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse password minimum length: %v", err)
	}
	lowerReq, err := strconv.ParseBool(envOrDefault(passwordLowerEnv, passwordLowerDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse lower case requirement: %v", err)
	}
	upperReq, err := strconv.ParseBool(envOrDefault(passwordUpperEnv, passwordUpperDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse upper case requirement: %v", err)
	}
	digit, err := strconv.ParseBool(envOrDefault(passwordDigitEnv, passwordDigitDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse digit requirement: %v", err)
	}
	symbol, err := strconv.ParseBool(envOrDefault(passwordSymbolEnv, passwordSymbolDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse special symbol requirement: %v", err)
	}

	mfaTOPT, err := strconv.ParseBool(envOrDefault(mfaTOTPEnv, mfaTOTPDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse MFA TOTP: %v", err)
	}
	mfaCertificate, err := strconv.ParseBool(envOrDefault(mfaTOTPEnv, mfaCertificateDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse MFA TOTP: %v", err)
	}
	var mfaAuthTypes []mfa.AuthenticatorType
	if mfaTOPT {
		mfaAuthTypes = append(mfaAuthTypes, mfa.TOTP)
	}
	if mfaCertificate {
		mfaAuthTypes = append(mfaAuthTypes, mfa.CERT)
	}

	reinitCert, err := strconv.ParseBool(envOrDefault(reinitCertProviderEnv, reinitCertProviderDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse new CA option: %v", err)
	}

	var feats FeaturesData
	feats, err = RetrieveFeatures("", "")
	if err != nil {
		return nil, fmt.Errorf("failed to validate the features: %v", err)
	}

	var ldapSyncInterval time.Duration
	if feats.LDAP() {
		ldapSyncInterval, err = time.ParseDuration(envOrDefault(ldapSyncIntervalEnv, ldapSyncIntervalDefault))
		if err != nil {
			return nil, fmt.Errorf("failed to parse ldap sync interval duration: %v", err)
		}
		if ldapSyncInterval == zeroDuration {
			return nil, fmt.Errorf("ldap sync interval duration must be non-zero")
		}
	}

	tokenExpiration, err := time.ParseDuration(envOrDefault(authTokenExpirationEnv, authTokenExpirationDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse auth token expiration duration: %v", err)
	}

	sessionExpiration, err := time.ParseDuration(envOrDefault(sessionExpirationEnv, sessionExpirationDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse session expiration duration: %v", err)
	}

	sessionExpCheckInterval, err := time.ParseDuration(envOrDefault(sessionExpCheckIntervalEnv, sessionExpCheckIntervalDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to parse session expiration check interval duration: %v", err)
	}
	if sessionExpCheckInterval == zeroDuration {
		return nil, fmt.Errorf("session expiration check interval duration must be non-zero"+
			" (if you want to disable session expiration, set %s to zero)", sessionExpirationEnv)
	}

	pwdCfg := &PasswordConfig{
		MinLength: pwdMinLen,
		Lowercase: lowerReq,
		Uppercase: upperReq,
		Symbol:    symbol,
		Digit:     digit,
	}

	restCfg := &RESTServerConfig{
		AuthType:        authType,
		Port:            restPort,
		Features:        feats,
		TokenExpiration: tokenExpiration,
		CertFile:        rpcCert,
		KeyFile:         rpcKey,
		TokensPerSecond: restTps,
		BucketSize:      restBucketSize,
	}
	rpcCfg := &RPCServerConfig{
		PortVpnS:        rpcPortVpnS,
		PortVpnC:        rpcPortVpnC,
		PortHealthcheck: rpcPortHealthcheck,
		CertFile:        rpcCert,
		KeyFile:         rpcKey,
		TokensPerSecond: grpcTps,
		BucketSize:      grpcBucketSize,
		Features:        feats,
	}

	pgCfg, err := PostgresConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to create DB config: %w", err)
	}

	vcmCfg := &VCMConfig{
		ReinitCert: reinitCert,
		Features:   feats,
	}

	usrCfg := &UserManagementConfig{
		LdapSyncInterval:        ldapSyncInterval,
		PwdCfg:                  pwdCfg,
		Features:                feats,
		MFAEnabledTypes:         mfaAuthTypes,
		HealthcheckRPCPortVpnC:  healthcheckRPCPortVpnC,
		SessionExpiration:       sessionExpiration,
		SessionExpCheckInterval: sessionExpCheckInterval,
	}

	c := &Config{
		REST:              restCfg,
		RPC:               rpcCfg,
		VCM:               vcmCfg,
		DB:                pgCfg,
		UserManager:       usrCfg,
		LogLevel:          logLevel,
		Features:          feats,
		EncryptionKeyPath: encryptionKeyPath,
	}
	return c, nil
}

func (rsc *RPCServerConfig) GetTLSCert() (*tls.Certificate, error) {
	if rsc.CertFile == "" {
		return nil, errors.New("missing path to a cert file")
	}
	if rsc.KeyFile == "" {
		return nil, errors.New("missing path to a key file")
	}

	cert, err := tls.LoadX509KeyPair(rsc.CertFile, rsc.KeyFile)
	if err != nil {
		return nil, err
	}

	return &cert, nil
}

func ReadEncryptionKey(encryptionKeyPath string) ([]byte, error) {
	if encryptionKeyPath == "" {
		return nil, errors.New("missing path to encryption key file")
	}

	encryptionKey, err := aesgcm.ReadKeyFromFile(encryptionKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read encryption key: %v", err)
	}
	return encryptionKey, nil
}

func envOrDefault(key string, defalt string) string {
	val := os.Getenv(key)
	if val == "" {
		log.WithFields(log.Fields{
			"variable": key,
			"value":    defalt,
		}).Info("Env variable was not set, setting the default value")
		val = defalt
	}

	return val
}
