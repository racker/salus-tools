/*
 * Copyright 2019 Rackspace US, Inc.
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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/url"
	"time"
)

const authTimeout = 60 * time.Second

type ClientAuthenticator interface {
	// PrepareRequest takes a client request and injects authentication headers, etc
	PrepareRequest(req *http.Request) error
}

type DisabledAuthenticator struct{}

func (DisabledAuthenticator) PrepareRequest(req *http.Request) error {
	// no-op
	return nil
}

type IdentityAuthenticator struct {
	IdentityUrl string
	Username    string
	Password    string
	Apikey      string

	log *zap.SugaredLogger

	token           string
	tokenExpiration time.Time
}

func NewIdentityAuthenticator(log *zap.SugaredLogger, identityUrl string, username string, password string, apikey string) *IdentityAuthenticator {
	return &IdentityAuthenticator{
		IdentityUrl: identityUrl,
		Username:    username,
		Password:    password,
		Apikey:      apikey,
		log:         log.Named("auth.identity"),
	}
}

type identityAuthApikeyReq struct {
	Auth struct {
		Credentials struct {
			Username string `json:"username"`
			Apikey   string `json:"apiKey"`
		} `json:"RAX-KSKEY:apiKeyCredentials"`
	} `json:"auth"`
}

type identityAuthPasswordReq struct {
	Auth struct {
		Credentials struct {
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"passwordCredentials"`
	} `json:"auth"`
}

// identityAuthResp only picks out the fields needed and ignores the majority of response content
type identityAuthResp struct {
	Access struct {
		Token struct {
			Id      string
			Expires time.Time
		}
	}
}

func (a *IdentityAuthenticator) PrepareRequest(req *http.Request, next RestClientNext) (*http.Response, error) {
	if time.Now().After(a.tokenExpiration) {
		if err := a.authenticate(); err != nil {
			return nil, err
		}
	}

	req.Header.Set("x-auth-token", a.token)

	return next(req)
}

func (a *IdentityAuthenticator) authenticate() error {
	if a.IdentityUrl == "" {
		return errors.New("IdentityUrl is required")
	}
	if a.Username == "" {
		return errors.New("Username is required")
	}
	if a.Password == "" && a.Apikey == "" {
		return errors.New("Password or Apikey is required")
	}

	var req interface{}
	if a.Apikey != "" {
		auth := &identityAuthApikeyReq{}
		auth.Auth.Credentials.Username = a.Username
		auth.Auth.Credentials.Apikey = a.Apikey
		req = auth
	} else {
		auth := &identityAuthPasswordReq{}
		auth.Auth.Credentials.Username = a.Username
		auth.Auth.Credentials.Password = a.Password
		req = auth
	}

	reqJson, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal identity request: %w", err)
	}

	baseUrl, err := url.Parse(a.IdentityUrl)
	if err != nil {
		return fmt.Errorf("failed to parse Identity URL: %w", err)
	}

	reqUrl, err := baseUrl.Parse("/v2.0/tokens")
	if err != nil {
		return fmt.Errorf("failed to build tokens request URL: %w", err)
	}

	reqCtx, _ := context.WithDeadline(context.Background(), time.Now().Add(authTimeout))
	request, err := http.NewRequestWithContext(reqCtx,
		"POST", reqUrl.String(), bytes.NewBuffer(reqJson))
	if err != nil {
		return fmt.Errorf("failed to build auth request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	a.log.Debugw("authenticating with Identity",
		"user", a.Username,
		"endpoint", a.IdentityUrl)
	postResp, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("failed to issue token request: %w", err)
	}
	//noinspection GoUnhandledErrorResult
	defer postResp.Body.Close()

	if postResp.StatusCode != 200 {
		var respBuf bytes.Buffer
		_, _ = io.Copy(&respBuf, postResp.Body)
		return fmt.Errorf("token request failed: (%d) %s", postResp.StatusCode, respBuf.String())
	}

	respDecoder := json.NewDecoder(postResp.Body)
	var resp identityAuthResp
	err = respDecoder.Decode(&resp)
	if err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	a.token = resp.Access.Token.Id
	a.tokenExpiration = resp.Access.Token.Expires

	return nil
}
