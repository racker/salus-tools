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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yalp/jsonpath"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestLoaderImpl_LoadAll(t *testing.T) {
	requests := make([]*http.Request, 0)
	postedJson := make([]interface{}, 0)

	// A lot of setup for latching requests and returning canned responses
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent-releases", func(w http.ResponseWriter, r *http.Request) {
		t.Log("handling agent-releases request", r.Method, r.URL.Path)
		requests = append(requests, r)

		w.WriteHeader(http.StatusOK)

		switch r.Method {
		case "GET":
			page := r.URL.Query().Get("page")
			if page == "" {
				page = "0"
			}
			file, err := os.Open(fmt.Sprintf("testdata/admin_agentRelease_p%s_resp.json", page))
			require.NoError(t, err)
			defer file.Close()
			io.Copy(w, file)

		case "POST":
			decoder := json.NewDecoder(r.Body)
			var parsed interface{}
			err := decoder.Decode(&parsed)
			require.NoError(t, err)
			postedJson = append(postedJson, parsed)
		}
	})
	mux.HandleFunc("/api/monitor-translations", func(w http.ResponseWriter, r *http.Request) {
		t.Log("handling monitor-translations request", r.Method, r.URL.Path)
		requests = append(requests, r)

		switch r.Method {
		case "GET":
			file, err := os.Open("testdata/admin_monitorTranslations_resp.json")
			require.NoError(t, err)
			defer file.Close()
			io.Copy(w, file)

		case "POST":
			decoder := json.NewDecoder(r.Body)
			var parsed interface{}
			err := decoder.Decode(&parsed)
			require.NoError(t, err)
			postedJson = append(postedJson, parsed)
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	loader, err := NewLoader(zap.NewNop().Sugar(), nil, ts.URL)
	require.NoError(t, err)

	// Finally...execute method under test

	stats, err := loader.LoadAll("testdata/content")
	require.NoError(t, err)

	assert.Len(t, requests, 5)

	// GET page 0 of agent releases
	i := 0
	assert.Equal(t, "GET", requests[i].Method)
	assert.Equal(t, "/api/agent-releases?page=0", requests[i].URL.String())

	// GET page 1 of agent releases
	i++
	assert.Equal(t, "GET", requests[i].Method)
	assert.Equal(t, "/api/agent-releases?page=1", requests[i].URL.String())

	// POST missing linux 1.11.0 agent release
	i++
	assert.Equal(t, "POST", requests[i].Method)
	assert.Equal(t, "/api/agent-releases", requests[i].URL.String())

	// GET monitor translations
	i++
	assert.Equal(t, "GET", requests[i].Method)
	assert.Equal(t, "/api/monitor-translations?page=0", requests[i].URL.String())

	// POST missing monitor translation
	i++
	assert.Equal(t, "POST", requests[i].Method)
	assert.Equal(t, "/api/monitor-translations", requests[i].URL.String())

	assert.Len(t, postedJson, 2)

	assertJsonPath(t, postedJson[0], "$.labels.agent_discovered_os", "darwin")
	assertJsonPath(t, postedJson[0], "$.type", "TELEGRAF")
	assertJsonPath(t, postedJson[0], "$.version", "1.11.0")

	assertJsonPath(t, postedJson[1], "$.name", "testing-definition")

	assert.NotNil(t, stats)
	assert.Equal(t, 2, stats.Created)
	assert.Equal(t, 1, stats.SkippedExisting)
	assert.Equal(t, 0, stats.FailedToCreate)
}

func TestLoaderDefinitions_validUniqueFieldPaths(t *testing.T) {
	for _, definition := range loaderDefinitions {
		assert.NotEmpty(t, definition.UniqueFieldPaths,
			"%s must have non-empty unique field paths", definition.Name)
	}
}

func TestLoaderImpl_identifyExistingContent_emptyUniqueFieldPaths(t *testing.T) {
	loader := &LoaderImpl{}
	content := make([]interface{}, 0)
	_, err := loader.identifyExistingContent(LoaderDefinition{
		Name:             "testing",
		ApiPath:          "/api/testing",
		UniqueFieldPaths: []string{},
	}, content)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "UniqueFieldPaths cannot be empty")
}

func TestLoaderImpl_identifyExistingContent_nilUniqueFieldPaths(t *testing.T) {
	loader := &LoaderImpl{}
	content := make([]interface{}, 0)
	_, err := loader.identifyExistingContent(LoaderDefinition{
		Name:             "testing",
		ApiPath:          "/api/testing",
		UniqueFieldPaths: nil,
	}, content)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "UniqueFieldPaths cannot be empty")
}

func assertJsonPath(t *testing.T, postedJson interface{}, path string, expected interface{}) {
	field, err := jsonpath.Read(postedJson, path)
	require.NoError(t, err)
	assert.Equal(t, expected, field)
}
