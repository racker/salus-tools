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
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultRestClientTimeout = 60 * time.Second

type RestClient struct {
	BaseUrl      *url.URL
	Timeout      time.Duration
	interceptors *list.List
}

type RestClientNext func(req *http.Request) (*http.Response, error)

type RestClientInterceptor func(req *http.Request, next RestClientNext) (*http.Response, error)

type FailedResponse struct {
	StatusCode int
	Content    []byte
}

func (r *FailedResponse) Error() string {
	return fmt.Sprintf("(%d) %s", r.StatusCode, string(r.Content))
}

func NewRestClient() *RestClient {
	return &RestClient{
		interceptors: list.New(),
	}
}

func (c *RestClient) AddInterceptor(it RestClientInterceptor) {
	c.interceptors.PushBack(it)
}

func (c *RestClient) Call(ctx context.Context, method string,
	urlIn string, query url.Values,
	body interface{}, respOut interface{}) error {

	var reqUrl *url.URL
	if c.BaseUrl != nil {
		var err error
		reqUrl, err = c.BaseUrl.Parse(urlIn)
		if err != nil {
			return fmt.Errorf("failed to parse given url relative to base: %w", err)
		}
	} else {
		var err error
		reqUrl, err = url.Parse(urlIn)
		if err != nil {
			return fmt.Errorf("filed to parse given url %s: %w", urlIn, err)
		}
	}

	if len(query) > 0 {
		reqUrl.RawQuery = query.Encode()
	}

	var bodyReader io.Reader
	if body == nil {
		bodyReader = nil
	} else if b, ok := body.([]byte); ok {
		bodyReader = bytes.NewBuffer(b)
	} else if s, ok := body.(string); ok {
		bodyReader = bytes.NewBufferString(s)
	} else {
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		err := encoder.Encode(body)
		if err != nil {
			return fmt.Errorf("failed to encode body: %w", err)
		}
		bodyReader = &buffer
	}

	timeoutCtx, cancelFunc := context.WithTimeout(ctx, c.timeout())
	defer cancelFunc()

	req, err := http.NewRequestWithContext(timeoutCtx, method, reqUrl.String(), bodyReader)
	if err != nil {
		return fmt.Errorf("failed to setup request: %w", err)
	}

	resp, err := c.doRequest(req, c.interceptors.Front())
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode >= 300 {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, resp.Body)
		_ = resp.Body.Close()
		return &FailedResponse{StatusCode: resp.StatusCode, Content: buffer.Bytes()}
	}

	if rs, ok := respOut.(*string); ok {
		var buffer bytes.Buffer
		_, err = io.Copy(&buffer, resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		*rs = buffer.String()
	} else if rb, ok := respOut.(*bytes.Buffer); ok {
		_, err := io.Copy(rb, resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
	} else if respOut != nil {
		decoder := json.NewDecoder(resp.Body)
		err := decoder.Decode(respOut)
		if err != nil {
			_ = resp.Body.Close()
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	err = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to close response body: %w", err)
	}
	return nil
}

func (c *RestClient) doRequest(req *http.Request, itElem *list.Element) (*http.Response, error) {

	if itElem == nil {
		return http.DefaultClient.Do(req)
	} else {
		// use unchecked cast since we force value types via AddInterceptor
		it := itElem.Value.(RestClientInterceptor)
		response, err := it(req, func(newReq *http.Request) (*http.Response, error) {
			return c.doRequest(newReq, itElem.Next())
		})
		if err != nil {
			return nil, err
		} else {
			return response, err
		}
	}
}

func (c *RestClient) timeout() time.Duration {
	if c.Timeout != 0 {
		return c.Timeout
	} else {
		return defaultRestClientTimeout
	}
}
