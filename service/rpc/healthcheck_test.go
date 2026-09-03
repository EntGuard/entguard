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

package rpc_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sqlc "github.com/entguard/entguard/db"
	"github.com/entguard/entguard/pkg/test"
	pbapi "github.com/entguard/entguard/proto/v2"
)

func TestIntegrationGetAllSessions(t *testing.T) {
	ctx := context.Background()
	test.SkipOrSetupIntegrationTest(t, ctx, q)

	// preconditions
	sqlcQ := sqlc.New(q.GetDbConnection(ctx))

	user1 := test.InsertSampleUser(t, ctx, q, "user1")
	user2 := test.InsertSampleUser(t, ctx, q, "user2")
	user3 := test.InsertSampleUser(t, ctx, q, "user3")

	addressPool := test.InsertSampleAddressPool(t, ctx, sqlcQ, test.NewSampleInsertAddressPoolParams("test-pool"))

	deviceTemplate1 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user1.ID, []int{addressPool.ID})
	deviceTemplate2 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user2.ID, []int{addressPool.ID})
	deviceTemplate3 := test.InsertSampleDeviceTemplate(t, ctx, sqlcQ, user3.ID, []int{addressPool.ID})

	_, session1 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate1.ID, nil)
	require.NotEmpty(t, session1)

	_, session2 := test.InsertSampleDeviceWithSession(t, ctx, sqlcQ, deviceTemplate2.ID, nil)
	require.NotEmpty(t, session2)

	// user3 device without session
	_ = test.InsertSampleDeviceManual(t, ctx, sqlcQ, deviceTemplate3.ID)

	expectedSessions := map[string]bool{
		session1: true,
		session2: true,
	}

	response, err := healthcheckServer.GetAllSessions(ctx, &pbapi.GetAllSessionsRequest{})
	require.NoError(t, err)
	require.NotEmpty(t, response)
	sessions := response.GetSessions()
	require.Len(t, sessions, len(expectedSessions))

	for _, s := range response.Sessions {
		require.True(t, expectedSessions[s])
	}
}
