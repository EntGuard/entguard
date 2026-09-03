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

package test

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/entguard/entguard/pkg/httpconst"
	"github.com/entguard/entguard/service/api/v1"
)

const (
	TestOrchestratorPort = "8180"
	TestOrchestratorURL  = "127.0.0.1:8180"
)

func CreateRequest(method, url string, body any, u *api.User) (*http.Request, error) {
	var bodyBuf bytes.Buffer
	switch v := body.(type) {
	case []byte:
		bodyBuf = *bytes.NewBuffer(v)
	default:
		err := json.NewEncoder(&bodyBuf).Encode(body)
		if err != nil {
			return nil, err
		}
	}
	url = httpconst.ProtocolHTTPS + TestOrchestratorURL + url
	req, err := http.NewRequest(method, url, &bodyBuf)
	if err != nil {
		return nil, fmt.Errorf("newRequest: %w", err)
	}
	req.Header.Set(httpconst.ContentType, httpconst.ApplicationJSON)

	token, err := FetchJWT(u.Username, TestUserPassword, TestOrchestratorURL)
	if err != nil {
		return nil, fmt.Errorf("FetchJWT: %w", err)
	}
	req.Header.Set(httpconst.Authorization, fmt.Sprintf("Bearer %s", token.Token))

	return req, nil
}

func FetchJWT(username, password, orchestratorURL string) (*api.JWT, error) {
	creds := api.JWTRequestCreds{
		Username: username,
		Password: password,
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	umarshaledCreds, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		httpconst.ProtocolHTTPS+orchestratorURL+api.GetJWTURL,
		bytes.NewBuffer(umarshaledCreds),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set(httpconst.ContentType, httpconst.ApplicationJSON)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching JWT token from orchestrator failed with status code: [%d] %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	jwt := &api.JWT{}
	if err = json.Unmarshal(body, jwt); err != nil {
		return nil, err
	}

	return jwt, nil
}

func URLWithID(url string, id string) string {
	return strings.Replace(url, ":id", id, 1)
}
