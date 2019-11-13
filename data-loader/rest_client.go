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

// RestClient provides a high-order implementation around Go's http.Request by incorporating
// relative URL building,
// timeout management,
// JSON request encoding,
// JSON response decoding,
// and non-2xx response status handling
type RestClient struct {
	BaseUrl      *url.URL
	Timeout      time.Duration
	interceptors *list.List
}

// RestClientNext is the callback type that will be provided to implementations of RestClientInterceptor
type RestClientNext func(req *http.Request) (*http.Response, error)

// RestClientInterceptor can be implemented to modify or replace an outgoing request and/or
// modify or replace the returned response. Implementations **must** invoke the next function.
//
// If processing only the outgoing request, then the interceptor can simply return the values of
// the call to next, such as
//
// return next(req)
type RestClientInterceptor func(req *http.Request, next RestClientNext) (*http.Response, error)

// FailedResponseError indicates that the server responded, but with a non-2xx status code
type FailedResponseError struct {
	StatusCode int
	Status     string
	Entity     *RestEntity
}

func (r *FailedResponseError) Error() string {
	// if []byte content then truncate and include in error
	if r.Entity != nil {
		if b, ok := r.Entity.Content.([]byte); ok {
			if len(b) > 100 {
				b = b[:100]
			}
			return fmt.Sprintf("%s body=[%s]", r.Status, string(b))
		}
	}
	// otherwise, just the status (which includes the code value)
	return r.Status
}

func NewRestClient() *RestClient {
	return &RestClient{}
}

func (c *RestClient) AddInterceptor(it RestClientInterceptor) {
	if c.interceptors == nil {
		c.interceptors = list.New()
	}
	c.interceptors.PushBack(it)
}

func (c *RestClient) SetBaseUrl(rawurl string) error {
	url, err := url.Parse(rawurl)
	if err != nil {
		return fmt.Errorf("failed to parse given base url: %w", err)
	}
	c.BaseUrl = url
	return nil
}

type MimeType string

const (
	JsonType MimeType = "application/json"
)

const (
	HeaderContentType = "Content-Type"
	HeaderAccept      = "Accept"
)

type RestEntity struct {
	ContentType MimeType
	Content     interface{}
}

func NewJsonEntity(content interface{}) *RestEntity {
	return &RestEntity{
		ContentType: JsonType,
		Content:     content,
	}
}

// Exchange prepares an HTTP request with optional JSON encoding,
// sends the request, and optionally processes the response with JSON decoding.
//
// The urlIn is either parsed relative to the BaseUrl configured on the client instance or parsed as is.
//
// If given, the query values are encoded into the final request URL.
//
// If body is non-nil, it will be used as the request payload.
// If body is a []byte, the content will be used as is,
// but any other type combined with a contentType of JsonType will be encoded as JSON.
//
// If respOut is non-nil, the response body will be placed at the given location. If a *string or *byte.Buffer
// is given, then the raw response body is used. Any other type combined with an accept of JsonType will
// be decoded as JSON.
//
// If the far-end responded with a non-2xx status code, then the returned error will be a
// FailedResponseError, which conveys the status code and response body's content.
func (c *RestClient) Exchange(method string,
	urlIn string, query url.Values,
	reqIn *RestEntity,
	respOut *RestEntity) error {
	return c.ExchangeWithContext(nil, method, urlIn, query, reqIn, respOut)
}

// ExchangeWithContext is the same as Exchange, but allows for a context to be provided
// to derive the request timeout context.
func (c *RestClient) ExchangeWithContext(ctx context.Context, method string,
	urlIn string, query url.Values,
	reqIn *RestEntity,
	respOut *RestEntity) error {

	reqUrl, err := c.buildReqUrl(urlIn, query)
	if err != nil {
		return err
	}

	bodyReader, err := c.buildBodyReader(reqIn)
	if err != nil {
		return err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	timeoutCtx, cancelFunc := context.WithTimeout(ctx, c.timeout())
	defer cancelFunc()

	req, err := c.buildRequest(timeoutCtx, method, reqUrl, bodyReader, reqIn, respOut)
	if err != nil {
		return err
	}

	var firstInterceptor *list.Element = nil
	if c.interceptors != nil {
		firstInterceptor = c.interceptors.Front()
	}
	resp, err := c.doRequest(req, firstInterceptor)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode >= 300 {
		// also closes the response body
		return c.buildFailedResponseError(resp)
	}

	if respOut != nil {
		err := c.processResponseContent(respOut, resp)
		if err != nil {
			_ = resp.Body.Close()
			return err
		}
	}

	err = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to close response body: %w", err)
	}
	return nil
}

func (c *RestClient) buildReqUrl(urlIn string, query url.Values) (*url.URL, error) {
	var reqUrl *url.URL
	if c.BaseUrl != nil {
		var err error
		reqUrl, err = c.BaseUrl.Parse(urlIn)
		if err != nil {
			return nil, fmt.Errorf("failed to parse given url relative to base: %w", err)
		}
	} else {
		var err error
		reqUrl, err = url.Parse(urlIn)
		if err != nil {
			return nil, fmt.Errorf("filed to parse given url %s: %w", urlIn, err)
		}
	}
	if len(query) > 0 {
		reqUrl.RawQuery = query.Encode()
	}
	return reqUrl, nil
}

func (c *RestClient) buildBodyReader(reqIn *RestEntity) (io.Reader, error) {
	var bodyReader io.Reader
	if reqIn == nil {
		bodyReader = nil
	} else if b, ok := reqIn.Content.([]byte); ok {
		bodyReader = bytes.NewBuffer(b)
	} else if reqIn.ContentType == JsonType && reqIn.Content != nil {
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		err := encoder.Encode(reqIn.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to encode body: %w", err)
		}
		bodyReader = &buffer
	} else {
		return nil, fmt.Errorf("unsupported combination of request content and type")
	}
	return bodyReader, nil
}

func (c *RestClient) buildRequest(timeoutCtx context.Context, method string, reqUrl *url.URL,
	bodyReader io.Reader, reqIn *RestEntity, respOut *RestEntity) (*http.Request, error) {
	req, err := http.NewRequestWithContext(timeoutCtx, method, reqUrl.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to setup request: %w", err)
	}
	if reqIn != nil && reqIn.ContentType != "" {
		req.Header.Set(HeaderContentType, string(reqIn.ContentType))
	}
	if respOut != nil && respOut.ContentType != "" {
		req.Header.Set(HeaderAccept, string(respOut.ContentType))
	}
	return req, nil
}

func (c *RestClient) processResponseContent(respOut *RestEntity, resp *http.Response) error {
	if rs, ok := respOut.Content.(*string); ok {
		var buffer bytes.Buffer
		_, err := io.Copy(&buffer, resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		*rs = buffer.String()
	} else if rb, ok := respOut.Content.(*bytes.Buffer); ok {
		_, err := io.Copy(rb, resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
	} else if respOut.ContentType == JsonType && respOut.Content != nil {
		decoder := json.NewDecoder(resp.Body)
		err := decoder.Decode(respOut.Content)
		if err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	} else {
		return fmt.Errorf("unsupported combination of request content reference and type")
	}
	return nil
}

func (c *RestClient) buildFailedResponseError(resp *http.Response) error {
	var buffer bytes.Buffer
	_, _ = io.Copy(&buffer, resp.Body)
	_ = resp.Body.Close()
	return &FailedResponseError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Entity: &RestEntity{
			ContentType: MimeType(resp.Header.Get(HeaderContentType)),
			Content:     buffer.Bytes(),
		},
	}
}

// doRequest will recursively process the interceptors via the position in the list conveyed by itElem
// and when itElem is nil the actual request is issued
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
