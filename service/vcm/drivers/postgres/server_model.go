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

package postgres

import (
	"errors"
	"fmt"
	"net/netip"

	"github.com/entguard/entguard/service/api/v1"
)

const (
	// maxServerNameLen defines length limit for server name.
	maxServerNameLen = 50
	// maxServerDescrLen defines length limit for server description.
	maxServerDescrLen = 250
)

// List of validation errors.
var (
	ErrMissingServerName               = errors.New("server name must be set")
	ErrMissingServerEndpoint           = errors.New("server endpoint must be set")
	ErrTooLongServerName               = fmt.Errorf("server name cannot be longer than %d characters", maxServerNameLen)
	ErrTooLongServerDescr              = fmt.Errorf("server description cannot be longer than %d characters", maxServerDescrLen)
	ErrInvalidServerEndpoint           = errors.New("server endpoint is not valid IP address")
	ErrInvalidServerHealthCheckAddress = errors.New("server health check address is not valid IP address")
)

type Server struct {
	ID                 int
	Name               string
	Endpoint           netip.Addr  // is required, cannot be nil
	HealthCheckAddress *netip.Addr // is optional, can be nil
	Description        string
}

func NewServerFromAPI(s *api.ServerWithVPNConfig) (*Server, error) {
	if s.Name == "" {
		return nil, ErrMissingServerName
	}
	if s.Endpoint == "" {
		return nil, ErrMissingServerEndpoint
	}

	if len(s.Name) > maxServerNameLen {
		return nil, ErrTooLongServerName
	}
	if len(s.Description) > maxServerDescrLen {
		return nil, ErrTooLongServerDescr
	}

	endpoint, err := netip.ParseAddr(s.Endpoint)
	if err != nil {
		return nil, ErrInvalidServerEndpoint
	}

	healthCheckAddress, err := parseHealthcheckAddress(s.HealthCheckAddress)
	if err != nil {
		return nil, err
	}

	return &Server{
		Name:               s.Name,
		Endpoint:           endpoint,
		HealthCheckAddress: healthCheckAddress,
		Description:        s.Description,
	}, nil
}

func (s *Server) SetName(name string) error {
	if name == "" {
		return ErrMissingServerName
	}
	if len(name) > maxServerNameLen {
		return ErrTooLongServerName
	}
	s.Name = name
	return nil
}

func (s *Server) SetEndpoint(endpoint string) error {
	if endpoint == "" {
		return ErrMissingServerEndpoint
	}
	ep, err := netip.ParseAddr(endpoint)
	if err != nil {
		return ErrInvalidServerEndpoint
	}

	s.Endpoint = ep
	return nil
}

func (s *Server) SetHealthCheckAddress(address string) error {
	addr, err := parseHealthcheckAddress(address)
	if err != nil {
		return err
	}
	s.HealthCheckAddress = addr
	return nil
}

func (s *Server) SetDescription(description string) error {
	if len(description) > maxServerDescrLen {
		return ErrTooLongServerDescr
	}

	s.Description = description
	return nil
}

func (s *Server) ToAPI() api.Server {
	healthcheckAddr := ""
	if s.HealthCheckAddress != nil {
		healthcheckAddr = s.HealthCheckAddress.String()
	}
	return api.Server{
		ID:                 s.ID,
		Name:               s.Name,
		Endpoint:           s.Endpoint.String(),
		HealthCheckAddress: healthcheckAddr,
		Description:        s.Description,
	}
}

func parseHealthcheckAddress(address string) (*netip.Addr, error) {
	if address == "" {
		return nil, nil
	}

	addr, err := netip.ParseAddr(address)
	if err != nil {
		return nil, ErrInvalidServerHealthCheckAddress
	}
	return &addr, nil
}
