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

package vpn_client

import (
	"context"

	log "github.com/sirupsen/logrus"

	orch "github.com/entguard/entguard/cmd/healthcheck/orchestrator"
	pbapiv2 "github.com/entguard/entguard/proto/v2"
)

type HealthcheckClient struct {
	pbapiv2.UnimplementedHealthcheckClientServiceServer

	hc  *orch.Healthcheck
	log *log.Entry
}

func (m *HealthcheckClient) HealthcheckPing(ctx context.Context, req *pbapiv2.HealthcheckPingRequest) (*pbapiv2.HealthcheckPingResponse, error) {
	ok, err := m.hc.IsSessionInList(req.Session)
	if err != nil {
		m.log.
			WithField("session", req.Session).
			Errorf("IsSessionInList: %v", err)

		return &pbapiv2.HealthcheckPingResponse{
			Status: pbapiv2.HealthcheckPingResponse_STATUS_NOT_INITIALIZED_SESSIONS,
		}, nil
	}

	status := pbapiv2.HealthcheckPingResponse_STATUS_INVALID_SESSION
	if ok {
		status = pbapiv2.HealthcheckPingResponse_STATUS_VALID_SESSION
	}
	return &pbapiv2.HealthcheckPingResponse{
		Status: status,
	}, nil
}
