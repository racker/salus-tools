/*
 * Copyright 2020 Rackspace US, Inc.
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
	"github.com/racker/go-restclient"
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

type Loader interface {
	LoadAll(sourceContentPath string) (*LoaderStats, error)
}

type LoaderStats struct {
	SkippedExisting int
	Created         int
	FailedToCreate  int
}

type LoaderImpl struct {
	log        *zap.SugaredLogger
	restClient *restclient.Client
}

func setupAndLoad(config *Config, log *zap.SugaredLogger, sourceContent SourceContent) error {
	sourceContentPath, err := sourceContent.Prepare()
	if err != nil {
		return fmt.Errorf("unable to prepare source content: %w", err)
	}
	defer sourceContent.Cleanup()

	clientAuth, err := OptionalIdentityAuthenticator(log, config)
	if err != nil {
		return fmt.Errorf("failed to setup Identity auth: %w", err)
	}

	loader, err := NewLoader(log, clientAuth, config.AdminUrl)
	if err != nil {
		return fmt.Errorf("failed to create loader: %w", err)
	}

	_, err = loader.LoadAll(sourceContentPath)
	if err != nil {
		return fmt.Errorf("failed to perform all loading: %w", err)
	}

	return nil
}

func NewLoader(log *zap.SugaredLogger, identityAuthenticator restclient.Interceptor, adminUrl string) (Loader, error) {
	ourLogger := log.Named("loader")
	ourLogger.Debugw("Setting up loader",
		"adminUrl", adminUrl)

	restClient := restclient.NewClient()
	err := restClient.SetBaseUrl(adminUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid admin URL: %w", err)
	}
	restClient.Timeout = getterTimeout
	if identityAuthenticator != nil {
		restClient.AddInterceptor(identityAuthenticator)
	}

	return &LoaderImpl{
		log:        ourLogger,
		restClient: restClient,
	}, nil
}

func (l *LoaderImpl) LoadAll(sourceContentPath string) (*LoaderStats, error) {

	stats := &LoaderStats{}
	var err1 error

	for _, definition := range loaderDefinitions {
		err := l.load(definition, sourceContentPath, stats)
		if err != nil {
			l.log.Warnw("failed to process loader definition",
				"err", err,
				"definition", definition)
			//but continue with other definitions
			err1 = err
		}
	}

	l.log.Infow("loaded content", "stats", stats)

	return stats, err1
}

func (l *LoaderImpl) load(definition LoaderDefinition, sourceContentPath string, stats *LoaderStats) error {

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

	err = l.processSourceContent(definition, sourceContentPath, identifiers, stats)
	if err != nil {
		return fmt.Errorf("failed to process source content: %w", err)
	}

	return nil
}

func (l *LoaderImpl) retrieveExistingPagedContent(definition LoaderDefinition) ([]interface{}, error) {
	l.log.Debugw("loading all pages for definition",
		"definition", definition)

	var content []interface{}

	for page := 0; ; page++ {
		query := make(url.Values)
		query.Set("page", strconv.Itoa(page))

		var pagedContent PagedContent
		err := l.restClient.Exchange("GET", definition.ApiPath, query,
			nil, restclient.NewJsonEntity(&pagedContent))

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

func (l *LoaderImpl) identifyExistingContent(definition LoaderDefinition, allContent []interface{}) (UniquenessTracker, error) {
	// if UniqueFieldPaths is empty then the uniqueness key always becomes an empty string,
	// in which case, content will load only if a GET for existing content returns nothing. After
	// that point all existing content via GET will look like it has the same empty-string-key as
	// the content to be loaded and nothing will get loaded.
	if len(definition.UniqueFieldPaths) == 0 {
		return nil, fmt.Errorf("UniqueFieldPaths cannot be empty for %+v", definition)
	}
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

func (l *LoaderImpl) extractFieldValues(definition LoaderDefinition, content interface{}) ([]interface{}, error) {
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

func (l *LoaderImpl) processSourceContent(definition LoaderDefinition, sourceContentPath string,
	existing UniquenessTracker, stats *LoaderStats) error {

	err := filepath.Walk(filepath.Join(sourceContentPath, definition.Name),
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && filepath.Ext(path) == ".json" {
				err := l.processSourceContentFile(definition, existing, path, stats)
				if err != nil {
					return fmt.Errorf("failed to process source content file %s: %w", path, err)
				}
			} else if !info.IsDir() {
				l.log.Debugw("skipping non-JSON file", "path", path)
			} // else ignore directories
			return nil
		})
	if err != nil {
		return err
	}

	return nil
}

func (l *LoaderImpl) processSourceContentFile(definition LoaderDefinition, existing UniquenessTracker,
	path string, stats *LoaderStats) error {
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
			stats.FailedToCreate += 1
			// but continue with others since data loader can always be re-run to pick up missed ones
		} else {
			stats.Created += 1
		}
	} else {
		stats.SkippedExisting += 1
	}

	return nil
}

func (l *LoaderImpl) loadEntity(definition LoaderDefinition, sourceContent interface{}) error {
	err := l.restClient.Exchange("POST", definition.ApiPath, nil, restclient.NewJsonEntity(sourceContent), nil)
	if err != nil {
		return fmt.Errorf("failed to create entity: %w", err)
	}
	return nil
}
