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

package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/entguard/entguard/pkg/httpconst"
	"github.com/entguard/entguard/pkg/models"
	"github.com/entguard/entguard/service/api/v1"
)

var (
	// ErrNoAuth indicates that REST API call requires authentication, but none were provided
	ErrNoAuth = errors.New("user credentials are required, but were not provided")
)

// Client allows to make REST API calls to VPN orchestration service.
type Client struct {
	addr   string
	client *http.Client

	// BasicAuth
	username string
	password string

	// JWT
	jwt string
}

// New returns a new Client. The Client will use the provided address addr as target address to connect to.
// In case of nonempty caCertFilePath, the Client in the process of connecting also verifies the target's certificate
// by using provided CA certificate.
func New(addr string, caCertFilePath string) (*Client, error) {
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}

	var transport http.RoundTripper
	if caCertFilePath != "" {
		caCert, err := os.ReadFile(caCertFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read custom CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		// transport trusting only our custom CA certificate
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		}
	}

	c := &Client{
		addr: addr,
		client: &http.Client{
			Transport: transport,
		},
	}
	return c, nil
}

func (c *Client) UseJWTAuth(username, password string) error {
	creds := api.JWTRequestCreds{
		Username: username,
		Password: password,
	}

	c.username = username
	c.password = password
	jwt, err := MakeJSONRequest[api.JWT](c, http.MethodPost, api.GetJWTURL, &creds)
	if err != nil {
		return err
	}
	c.jwt = jwt.Token
	c.username = ""
	c.password = ""
	return nil
}

func (c *Client) UseBasicAuth(name, password string) {
	c.username = name
	c.password = password
}

func (c *Client) GetSecrets(servername string) (*models.Secrets, error) {
	secretsReq := api.VPNServerSecretsRequest{
		ServerName: servername,
	}

	return MakeJSONRequest[models.Secrets](
		c,
		http.MethodPost,
		api.VPNServerSecretsURL,
		&secretsReq,
	)
}

func MakeJSONRequest[T, U any, PT interface{ *T }](c *Client, method, path string, reqBody *U) (PT, error) {
	if c.jwt == "" && (c.password == "" || c.username == "") {
		return nil, ErrNoAuth
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	url := c.addr + path
	if !strings.Contains(c.addr, httpconst.ProtocolHTTPS) {
		url = httpconst.ProtocolHTTPS + url
	}
	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	if c.jwt != "" {
		req.Header.Set(httpconst.Authorization, "Bearer "+c.jwt)
	} else {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Add(httpconst.ContentType, httpconst.ApplicationJSON)
	req.Header.Add(httpconst.Accept, httpconst.ApplicationJSON)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respJSON, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	codeErr := fmt.Errorf("[%d] %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	if len(respJSON) == 0 {
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return nil, codeErr
		}
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errResp api.ErrorResponse
		if err := json.Unmarshal(respJSON, &errResp); err != nil {
			return nil, fmt.Errorf("unmarshaling error response: %w", err)
		}
		return nil, fmt.Errorf("%w: %s", codeErr, errResp.Message)
	}
	result := PT(new(T))
	if err := json.Unmarshal(respJSON, result); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}
	return result, nil
}
