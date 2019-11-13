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
	"github.com/yalp/jsonpath"
	"go.uber.org/zap"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const getterTimeout = 10 * time.Second

type LoaderDefinition struct {
	Name             string
	GetterPath       string
	NonPaged         bool
	UniqueFieldPaths []string
}

func (l *LoaderDefinition) String() string {
	return l.Name
}

var loaderDefinitions = []LoaderDefinition{
	{
		Name:       "agent-releases",
		GetterPath: "/api/agent-releases",
		UniqueFieldPaths: []string{
			"$.type",
			"$.version",
			"$.labels.agent_discovered_os",
			"$.labels.agent_discovered_arch",
		},
	},
}

type PagedContent struct {
	Content []interface{}
	Last    bool
}

type Loader struct {
	log               *zap.SugaredLogger
	restClient        *RestClient
	sourceContentPath string
}

func NewLoader(log *zap.SugaredLogger, identityAuthenticator *IdentityAuthenticator, adminUrl string, sourceContentPath string) (*Loader, error) {
	parsedAdminUrl, err := url.Parse(adminUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse adminUrl %s: %w", adminUrl, err)
	}

	ourLogger := log.Named("loader")
	ourLogger.Debugw("Setting up loader",
		"adminUrl", adminUrl)

	restClient := NewRestClient()
	restClient.BaseUrl = parsedAdminUrl
	restClient.Timeout = getterTimeout
	if identityAuthenticator != nil {
		restClient.AddInterceptor(identityAuthenticator.Intercept)
	}

	return &Loader{
		log:               ourLogger,
		restClient:        restClient,
		sourceContentPath: sourceContentPath,
	}, nil
}

func (l *Loader) LoadAll() error {
	for _, definition := range loaderDefinitions {
		err := l.load(definition)
		if err != nil {
			l.log.Errorw("failed to process loader definition",
				"err", err,
				"definition", definition)
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

	l.log.Infof("Loaded %d existing entities for %s", len(content), definition.Name)
	l.log.Debugw("Loaded existing content",
		"content", content,
		"definition", definition.Name)

	identifiers, err := l.identifyExistingContent(definition, content)
	if err != nil {
		return fmt.Errorf("failure while identifying existing content: %w", err)
	}

	l.log.Debugw("Identified existing content",
		"identifiers", identifiers,
		"definition", definition)

	// TODO
	// walk source content
	// read json files
	// resolve source content not already existing
	// call REST to create those

	return nil
}

func (l *Loader) loadAllPages(definition LoaderDefinition) ([]interface{}, error) {
	l.log.Debugw("loading all pages for definition",
		"definition", definition)

	var content []interface{}

	for page := 0; ; page++ {
		query := make(url.Values)
		query.Set("page", strconv.Itoa(page))

		var pagedContent PagedContent
		err := l.restClient.Exchange(context.Background(), "GET", definition.GetterPath, query,
			nil, NewJsonEntity(&pagedContent))

		if err != nil {
			return nil, fmt.Errorf("failed to get page %d of %s: %w", page, definition.Name, err)
		}

		content = append(content, pagedContent.Content...)

		if pagedContent.Last {
			break
		}
	}

	return content, nil
}

type UniquenessTracker map[string]struct{}

func (t UniquenessTracker) String() string {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	return fmt.Sprintf("[%s]", strings.Join(keys, ","))
}

func (t UniquenessTracker) Add(fieldValues []interface{}) {
	key := t.formKey(fieldValues)
	t[key] = struct{}{}
}

func (t UniquenessTracker) Contains(fieldValues []interface{}) bool {
	_, exists := t[t.formKey(fieldValues)]
	return exists
}

func (UniquenessTracker) formKey(fieldValues []interface{}) string {
	strValues := make([]string, len(fieldValues))
	for i, v := range fieldValues {
		strValues[i] = fmt.Sprintf("%v", v)
	}
	key := strings.Join(strValues, ";")
	return key
}

func (l *Loader) identifyExistingContent(definition LoaderDefinition, content []interface{}) (UniquenessTracker, error) {
	tracker := make(UniquenessTracker)

	for _, v := range content {
		fieldValues := make([]interface{}, 0, len(definition.UniqueFieldPaths))
		for _, path := range definition.UniqueFieldPaths {
			fieldValue, err := jsonpath.Read(v, path)
			if err != nil {
				return nil, fmt.Errorf("failed to read json path given content: %w", err)
			}
			fieldValues = append(fieldValues, fieldValue)
		}

		tracker.Add(fieldValues)
	}

	return tracker, nil
}
