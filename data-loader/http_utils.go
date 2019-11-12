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
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
)

// newFailedResponseError builds an error that conveys the status code and the body of response
// This function will take care of closing the response body
func newFailedResponseError(msg string, request *http.Request, response *http.Response) error {
	var respBuf bytes.Buffer
	_, _ = io.Copy(&respBuf, response.Body)
	closeResponse(request, response)
	return fmt.Errorf("%s: (%d) %s", msg, response.StatusCode, respBuf.String())
}

func closeResponse(request *http.Request, response *http.Response) {
	err := response.Body.Close()
	if err != nil {
		logrus.WithError(err).WithField("req", request).Warn("failed to close response body")
	}
}

func decodeResponse(request *http.Request, response *http.Response, v interface{}) error {
	decoder := json.NewDecoder(response.Body)
	err := decoder.Decode(v)
	if err != nil {
		return err
	}
	closeResponse(request, response)
	return nil
}
