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
	"context"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"net/http"
	"net/url"
	"time"
)

const authTimeout = 60 * time.Second

type IdentityAuthenticator struct {
	username string
	password string
	apikey   string

	log        *zap.SugaredLogger
	restClient *RestClient

	token           string
	tokenExpiration time.Time
}

func NewIdentityAuthenticator(log *zap.SugaredLogger, identityUrl string, username string, password string, apikey string) (*IdentityAuthenticator, error) {
	if username == "" {
		return nil, errors.New("username is required")
	}
	if password == "" && apikey == "" {
		return nil, errors.New("password or Apikey is required")
	}

	baseIdentityUrl, err := url.Parse(identityUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse given identityUrl: %w", err)
	}

	restClient := NewRestClient()
	restClient.BaseUrl = baseIdentityUrl
	restClient.Timeout = authTimeout

	return &IdentityAuthenticator{
		username:   username,
		password:   password,
		apikey:     apikey,
		log:        log.Named("auth.identity"),
		restClient: restClient,
	}, nil
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

func (a *IdentityAuthenticator) Intercept(req *http.Request, next RestClientNext) (*http.Response, error) {
	if time.Now().After(a.tokenExpiration) {
		if err := a.authenticate(); err != nil {
			return nil, err
		}
	}

	req.Header.Set("x-auth-token", a.token)

	return next(req)
}

func (a *IdentityAuthenticator) authenticate() error {

	var req interface{}
	if a.apikey != "" {
		auth := &identityAuthApikeyReq{}
		auth.Auth.Credentials.Username = a.username
		auth.Auth.Credentials.Apikey = a.apikey
		req = auth
	} else {
		auth := &identityAuthPasswordReq{}
		auth.Auth.Credentials.Username = a.username
		auth.Auth.Credentials.Password = a.password
		req = auth
	}

	var resp identityAuthResp

	a.log.Debugw("authenticating with Identity",
		"user", a.username,
		"endpoint", a.restClient.BaseUrl)
	err := a.restClient.Exchange(context.Background(), "POST", "/v2.0/tokens", nil,
		NewJsonEntity(req), NewJsonEntity(&resp))
	if err != nil {
		return fmt.Errorf("failed to issue token request: %w", err)
	}

	a.token = resp.Access.Token.Id
	a.tokenExpiration = resp.Access.Token.Expires

	return nil
}
