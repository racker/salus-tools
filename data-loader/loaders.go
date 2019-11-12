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
	restClient        *RestClient
	sourceContentPath string
}

func NewLoader(identityAuthenticator *IdentityAuthenticator, adminUrl string, sourceContentPath string) (*Loader, error) {
	parsedAdminUrl, err := url.Parse(adminUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse adminUrl %s: %w", adminUrl, err)
	}

	restClient := NewRestClient()
	restClient.BaseUrl = parsedAdminUrl
	if identityAuthenticator != nil {
		restClient.AddInterceptor(identityAuthenticator.PrepareRequest)
	}

	return &Loader{
		restClient:        restClient,
		sourceContentPath: sourceContentPath,
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

	var content []interface{}
	if definition.NonPaged {
		// todo
	} else {
		var err error
		content, err = l.loadAllPages(definition)
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

func (l *Loader) loadAllPages(definition LoaderDefinition) ([]interface{}, error) {
	var content []interface{}

	for page := 0; ; page++ {
		query := make(url.Values)
		query.Set("page", strconv.Itoa(page))

		var pagedContent PagedContent
		err := l.restClient.Call(context.Background(), "GET", definition.GetterPath, query,
			nil, &pagedContent)

		if err != nil {
			return nil, fmt.Errorf("failed to get page %d of %s: %w", page, definition.Name, err)
		}

		content = append(content, pagedContent.Content)

		if pagedContent.Last {
			break
		}
	}

	return content, nil
}
