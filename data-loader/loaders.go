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
	"fmt"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const getterTimeout = 10 * time.Second

type LoaderDefinition struct {
	Name       string
	GetterPath string
	NonPaged   bool
}

var loaderDefinitions = []LoaderDefinition{
	{
		Name:       "agent-releases",
		GetterPath: "/api/agent-releases",
	},
}

type PagedContent struct {
	Content []interface{}
	Last    bool
}

type Loader struct {
	clientAuthenticator ClientAuthenticator
	sourceContentPath   string
	adminUrl            *url.URL
}

func NewLoader(clientAuthenticator ClientAuthenticator, adminUrl string, sourceContentPath string) (*Loader, error) {
	parsedAdminUrl, err := url.Parse(adminUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse adminUrl %s: %w", adminUrl, err)
	}

	return &Loader{
		clientAuthenticator: clientAuthenticator,
		sourceContentPath:   sourceContentPath,
		adminUrl:            parsedAdminUrl,
	}, nil
}

func (l *Loader) LoadAll() error {
	for _, definition := range loaderDefinitions {
		err := l.load(definition)
		if err != nil {
			return fmt.Errorf("failed to process loader definition %+v: %w", definition, err)
		}
	}
	return nil
}

func (l *Loader) load(definition LoaderDefinition) error {

	getterUrl, err := l.adminUrl.Parse(definition.GetterPath)
	if err != nil {
		return fmt.Errorf("failed to build getter url: %w", err)
	}

	var content []interface{}
	if definition.NonPaged {
		// todo
	} else {
		content, err = l.loadAllPages(definition, getterUrl)
		if err != nil {
			return fmt.Errorf("failed to load all pages: %w", err)
		}
	}

	logrus.
		WithField("content", content).
		WithField("definition", definition.Name).
		Info("Loaded exiting content")

	return nil
}

func (l *Loader) loadAllPages(definition LoaderDefinition, getterUrl *url.URL) ([]interface{}, error) {
	var content []interface{}

	for page := 0; ; page++ {
		pageUrl := *getterUrl
		query := make(url.Values)
		query.Set("page", strconv.Itoa(page))
		pageUrl.RawQuery = query.Encode()

		reqCtx, _ := context.WithDeadline(context.Background(), time.Now().Add(getterTimeout))
		request, err := http.NewRequestWithContext(reqCtx, "GET", pageUrl.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build page request: %w", err)
		}

		err = l.clientAuthenticator.PrepareRequest(request)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare auth for page request: %w", err)
		}

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("failed to submit page request: %w", err)
		}

		if response.StatusCode != 200 {
			return nil, newFailedResponseError("page request failed", request, response)
		}

		var pagedContent PagedContent
		err = decodeResponse(request, response, &pagedContent)
		if err != nil {
			return nil, fmt.Errorf("failed to decode page %d of content: %w", err)
		}

		content = append(content, pagedContent.Content)

		if pagedContent.Last {
			break
		}
	}

	return content, nil
}
