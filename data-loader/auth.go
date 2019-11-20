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
	"errors"
	"fmt"
	"github.com/racker/go-restclient"
	"go.uber.org/zap"
	"net/http"
	"strings"
	"time"
)

const authTimeout = 60 * time.Second

type IdentityAuthenticator struct {
	username string
	password string
	apikey   string

	log        *zap.SugaredLogger
	restClient *restclient.Client

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

	restClient := restclient.New()
	err := restClient.SetBaseUrl(identityUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid Identity URL: %w", err)
	}
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

func (a *IdentityAuthenticator) Intercept(req *http.Request, next restclient.NextCallback) (*http.Response, error) {
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
	err := a.restClient.Exchange("POST", "/v2.0/tokens", nil,
		restclient.NewJsonEntity(req), restclient.NewJsonEntity(&resp))
	if err != nil {
		return fmt.Errorf("failed to issue token request: %w", err)
	}

	a.token = resp.Access.Token.Id
	a.tokenExpiration = resp.Access.Token.Expires

	return nil
}

func OptionalIdentityAuthenticator(log *zap.SugaredLogger, config *Config) (*IdentityAuthenticator, error) {
	var clientAuth *IdentityAuthenticator
	if !strings.Contains(config.AdminUrl, "localhost") {
		var err error
		clientAuth, err = NewIdentityAuthenticator(log,
			config.IdentityUrl, config.IdentityUsername, config.IdentityPassword, config.IdentityApikey)
		return nil, err
	}

	return clientAuth, nil
}
