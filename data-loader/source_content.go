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
	"github.com/sirupsen/logrus"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"io/ioutil"
	"os"
)

type SourceContent interface {
	// Prepare returns the path to the directory containing the prepared content or an error
	// if something went wrong
	Prepare() (string, error)
	Cleanup()
}

func NewSourceContentFromDir(dir string) SourceContent {
	return &dirSourceContent{
		dir: dir,
	}
}

type dirSourceContent struct {
	dir string
}

func (c *dirSourceContent) Prepare() (string, error) {
	logrus.
		WithField("dir", c.dir).
		Info("using source content from local directory")
	// just return the configured directory
	return c.dir, nil
}

func (c *dirSourceContent) Cleanup() {
	// no cleanup needed
}

func NewSourceContentFromGit(repository string, sha string, githubToken string) SourceContent {
	return &gitSourceContent{
		repository:  repository,
		sha:         sha,
		githubToken: githubToken,
	}
}

type gitSourceContent struct {
	repository  string
	sha         string
	workingDir  string
	githubToken string
}

func (c *gitSourceContent) Prepare() (string, error) {
	var err error
	c.workingDir, err = ioutil.TempDir("", "data-loader")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	var authMethod transport.AuthMethod = nil

	if c.githubToken != "" {
		authMethod = &http.BasicAuth{
			Username: "git",
			Password: c.githubToken,
		}
	}

	repo, err := git.PlainClone(c.workingDir, false, &git.CloneOptions{
		URL:  c.repository,
		Auth: authMethod,
	})
	if err != nil {
		return "", fmt.Errorf("failed to clone repo: %w", err)
	}

	if c.sha != "" {
		worktree, err := repo.Worktree()
		if err != nil {
			return "", fmt.Errorf("failed to access worktree: %w", err)
		}

		err = worktree.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(c.sha)})
		if err != nil {
			return "", fmt.Errorf("failed to checkout specific commit: %w", err)
		}
	} else {
		headRef, err := repo.Head()
		if err != nil {
			return "", fmt.Errorf("failed to resolve HEAD: %w", err)
		}

		c.sha = headRef.Hash().String()
	}

	logrus.WithField("repository", c.repository).
		WithField("sha", c.sha).
		Info("cloned source content")

	return c.workingDir, nil
}

func (c *gitSourceContent) Cleanup() {
	//noinspection GoUnhandledErrorResult
	os.RemoveAll(c.workingDir)
}