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

package grpc

import (
	"net"

	"google.golang.org/grpc"
)

func RunGRPCServer(s *grpc.Server, addr string, e chan error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		e <- err
		return
	}

	err = s.Serve(lis)
	if err != nil {
		e <- err
		return
	}
}
