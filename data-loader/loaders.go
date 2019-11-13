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
	"encoding/json"
	"fmt"
	"github.com/yalp/jsonpath"
	"go.uber.org/zap"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const getterTimeout = 10 * time.Second

type LoaderDefinition struct {
	Name             string
	ApiPath          string
	UniqueFieldPaths []string
}

func (l *LoaderDefinition) String() string {
	return l.Name
}

type PagedContent struct {
	Content []interface{}
	Last    bool
}

type Loader struct {
	log               *zap.SugaredLogger
	restClient        *RestClient
	sourceContentPath string
	stats             struct {
		SkippedExisting int
		Created         int
		FailedToCreate  int
	}
}

func NewLoader(log *zap.SugaredLogger, identityAuthenticator *IdentityAuthenticator, adminUrl string, sourceContentPath string) (*Loader, error) {
	ourLogger := log.Named("loader")
	ourLogger.Debugw("Setting up loader",
		"adminUrl", adminUrl)

	restClient := NewRestClient()
	err := restClient.SetBaseUrl(adminUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid admin URL: %w", err)
	}
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

func (l *Loader) LoadAll(sourceContentPath string) error {
	for _, definition := range loaderDefinitions {
		err := l.load(definition, sourceContentPath)
		if err != nil {
			l.log.Warnw("failed to process loader definition",
				"err", err,
				"definition", definition)
			// but continue with other definitions
		}
	}

	l.log.Infow("finishing loading content", "stats", l.stats)

	return nil
}

func (l *Loader) load(definition LoaderDefinition, sourceContentPath string) error {

	var content []interface{}
	var err error
	content, err = l.retrieveExistingPagedContent(definition)
	if err != nil {
		return fmt.Errorf("failed to load all pages: %w", err)
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

	err = l.processSourceContent(definition, sourceContentPath, identifiers)
	if err != nil {
		return fmt.Errorf("failed to process source content: %w", err)
	}

	return nil
}

func (l *Loader) retrieveExistingPagedContent(definition LoaderDefinition) ([]interface{}, error) {
	l.log.Debugw("loading all pages for definition",
		"definition", definition)

	var content []interface{}

	for page := 0; ; page++ {
		query := make(url.Values)
		query.Set("page", strconv.Itoa(page))

		var pagedContent PagedContent
		err := l.restClient.Exchange("GET", definition.ApiPath, query,
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

func (l *Loader) identifyExistingContent(definition LoaderDefinition, allContent []interface{}) (UniquenessTracker, error) {
	tracker := make(UniquenessTracker)

	for _, v := range allContent {
		fieldValues, e := l.extractFieldValues(definition, v)
		if e != nil {
			return nil, e
		}

		tracker.Add(fieldValues)
	}

	return tracker, nil
}

func (l *Loader) extractFieldValues(definition LoaderDefinition, content interface{}) ([]interface{}, error) {
	fieldValues := make([]interface{}, 0, len(definition.UniqueFieldPaths))
	for _, path := range definition.UniqueFieldPaths {
		fieldValue, err := jsonpath.Read(content, path)
		if err != nil {
			return nil, fmt.Errorf("failed to read json path given content: %w", err)
		}
		fieldValues = append(fieldValues, fieldValue)
	}
	return fieldValues, nil
}

func (l *Loader) processSourceContent(definition LoaderDefinition, sourceContentPath string, existing UniquenessTracker) error {

	err := filepath.Walk(filepath.Join(sourceContentPath, definition.Name), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			err := l.processSourceContentFile(definition, existing, path)
			if err != nil {
				return fmt.Errorf("failed to process source content file %s: %w", path, err)
			}
		} else {
			l.log.Debugw("skipping non-JSON file", "path", path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (l *Loader) processSourceContentFile(definition LoaderDefinition, existing UniquenessTracker, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open source content file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var sourceContent interface{}
	err = decoder.Decode(&sourceContent)
	if err != nil {
		return fmt.Errorf("failed to decode source content: %w", err)
	}

	fieldValues, err := l.extractFieldValues(definition, sourceContent)
	if err != nil {
		return fmt.Errorf("failed to extract unique fields values: %w", err)
	}

	if !existing.Contains(fieldValues) {
		l.log.Debugw("loading new entity from source content",
			"content", sourceContent, "path", path)
		err := l.loadEntity(definition, sourceContent)
		if err != nil {
			l.log.Errorw("failed to load new entity from source content",
				"err", err, "path", path)
			l.stats.FailedToCreate += 1
			// but continue with others since data loader can always be re-run to pick up missed ones
		} else {
			l.stats.Created += 1
		}
	} else {
		l.stats.SkippedExisting += 1
	}

	return nil
}

func (l *Loader) loadEntity(definition LoaderDefinition, sourceContent interface{}) error {
	err := l.restClient.Exchange("POST", definition.ApiPath, nil, NewJsonEntity(sourceContent), nil)
	if err != nil {
		return fmt.Errorf("failed to create entity: %w", err)
	}
	return nil
}
