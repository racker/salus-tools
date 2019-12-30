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
	"github.com/racker/go-restclient"
	"go.uber.org/zap"
	"strings"
)

// OptionalIdentityAuthenticator creates an IdentityAuthenticator instance only if the configured
// admin URL is remote.
func OptionalIdentityAuthenticator(log *zap.SugaredLogger, config *Config) (restclient.Interceptor, error) {
	if !strings.Contains(config.AdminUrl, "localhost") {
		clientAuth, err := restclient.IdentityV2Authenticator(
			config.IdentityUrl, config.IdentityUsername, config.IdentityPassword, config.IdentityApikey)
		return clientAuth, err
	}

	return nil, nil
}
