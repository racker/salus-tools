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
	"fmt"
	"github.com/google/go-github/v28/github"
	"go.uber.org/zap"
	"net/http"
	"reflect"
	"regexp"
)

type WebhookServer struct {
	log               *zap.SugaredLogger
	loader            Loader
	port              int
	gitContentBuilder GitSourceContentBuilder
	webhookSecret     []byte
	matchingRefs      []string
}

func NewWebhookServer(log *zap.SugaredLogger, loader Loader, port int, gitContentBuilder GitSourceContentBuilder, webhookSecret string, matchingRefs []string) *WebhookServer {
	ourLogger := log.Named("webhook")
	return &WebhookServer{
		log:               ourLogger,
		loader:            loader,
		port:              port,
		gitContentBuilder: gitContentBuilder,
		webhookSecret:     []byte(webhookSecret),
		matchingRefs:      matchingRefs,
	}
}

func (s *WebhookServer) Start() error {
	http.HandleFunc("/webhook", s.handleWebhook)

	// register healthcheck endpoint
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	s.log.Infow("webhook server running", "port", s.port)
	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), nil)
}

func (s *WebhookServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.log.Warnw("wrong method in webhook request",
			"method", r.Method, "remote", r.RemoteAddr)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	payload, err := github.ValidatePayload(r, s.webhookSecret)
	if err != nil {
		s.log.Warnw("failed to validate webhook payload", "err", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		s.log.Warnw("unable to parse webhook request", "err", err)
		s.writeErrResponse(http.StatusBadRequest, w, err)
		return
	}

	s.log.Debugw("received webhook event", "event", event)

	switch event := event.(type) {
	case *github.PushEvent:
		err := s.handlePushEvent(github.DeliveryID(r), event)
		if err != nil {
			s.log.Warnw("failed to handle push event", "err", err)
			s.writeErrResponse(http.StatusInternalServerError, w, err)
			return
		}

	default:
		s.log.Debugw("ignoring unsupported webhook event type",
			"type", reflect.ValueOf(event).Type())
	}
}

func (s *WebhookServer) writeErrResponse(statusCode int, w http.ResponseWriter, err error) {
	w.WriteHeader(statusCode)
	_, writeErr := w.Write([]byte(err.Error()))
	if writeErr != nil {
		s.log.Warnw("failed to write error response",
			"err", writeErr)
	}
}

func (s *WebhookServer) handlePushEvent(deliveryId string, event *github.PushEvent) error {
	ref := event.GetRef()
	pusher := event.GetPusher().GetName()
	cloneURL := event.GetRepo().GetCloneURL()
	commitId := event.GetHeadCommit().GetID()

	if !s.isApplicableRef(ref) {
		s.log.Debugw("ignoring push event ref that does not match",
			"ref", ref, "deliveryId", deliveryId)
		return nil
	}

	sourceContent := s.gitContentBuilder(cloneURL, commitId)

	sourceContentPath, err := sourceContent.Prepare()
	if err != nil {
		return fmt.Errorf("failed to prepare source content from %s: %w", cloneURL, err)
	}
	defer sourceContent.Cleanup()

	s.log.Infow("loading source content for webhook push event",
		"pusher", pusher, "ref", ref, "cloneURL", cloneURL, "commitId", commitId,
		"deliveryId", deliveryId)
	err = s.loader.LoadAll(sourceContentPath)
	if err != nil {
		return fmt.Errorf("failed load content: %w", err)
	}

	return nil
}

func (s *WebhookServer) isApplicableRef(ref string) bool {
	if len(s.matchingRefs) == 0 {
		// non-configured, so match any
		return true
	}

	for _, expr := range s.matchingRefs {
		matched, err := regexp.MatchString(expr, ref)
		if err != nil {
			s.log.Warnw("failed to process refs matching expression",
				"expr", expr, "ref", ref, "err", err)
			continue
		}
		if matched {
			return true
		}
	}

	return false
}

func nillableString(s *string, field string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("%s is not set", field)
	} else {
		return *s, nil
	}
}
