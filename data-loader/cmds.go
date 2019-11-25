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
	"flag"
	"fmt"
	"github.com/google/subcommands"
	"github.com/itzg/go-flagsfiller"
	"go.uber.org/zap"
	"log"
	"os"
)

type loadFromGitCmd struct {
	GithubToken string `usage:"access [token] for private Github repos" env:"GITHUB_TOKEN"`
	Sha         string `usage:"a specific commit SHA to check out"`
}

func (c *loadFromGitCmd) Name() string {
	return "load-from-git"
}

func (c *loadFromGitCmd) Synopsis() string {
	return "Loads content from a specific git repository"
}

func (c *loadFromGitCmd) Usage() string {
	return `load-from-git [flags] repositoryUrl
Flags:
`
}

func (c *loadFromGitCmd) SetFlags(f *flag.FlagSet) {
	filler := flagsfiller.New()
	err := filler.Fill(f, c)
	if err != nil {
		log.Fatal(err)
	}
}

func (c *loadFromGitCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	logger := args[0].(*zap.SugaredLogger)
	config := args[1].(*Config)

	if f.NArg() < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "missing repository URL")
		f.Usage()
		return subcommands.ExitUsageError
	}

	repoUrl := f.Arg(0)
	logger.Debugw("running load-from-git",
		"repo", repoUrl, "sha", c.Sha, "config", config)

	sourceContent := NewSourceContentFromGit(logger, repoUrl, c.Sha, c.GithubToken)

	err := setupAndLoad(config, logger, sourceContent)
	if err != nil {
		logger.Errorw("data loading failed", "err", err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

type loadFromLocalDirCmd struct {
}

func (c *loadFromLocalDirCmd) Name() string {
	return "load-from-local"
}

func (c *loadFromLocalDirCmd) Synopsis() string {
	return "Loads content from a local directory"
}

func (c *loadFromLocalDirCmd) Usage() string {
	return `load-from-local contentDirPath
`
}

func (c *loadFromLocalDirCmd) SetFlags(*flag.FlagSet) {
	// none to set
}

func (c *loadFromLocalDirCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	logger := args[0].(*zap.SugaredLogger)
	config := args[1].(*Config)

	if f.NArg() < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "missing content directory path")
		f.Usage()
		return subcommands.ExitUsageError
	}

	path := f.Arg(0)
	logger.Debugw("running load-from-local",
		"path", path, "config", config)

	sourceContent := NewSourceContentFromDir(logger, path)

	err := setupAndLoad(config, logger, sourceContent)
	if err != nil {
		logger.Errorw("data loading failed", "err", err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

type webhookServerCmd struct {
	Port          int      `usage:"the port where webhook server will bind" default:"8080"`
	GithubToken   string   `usage:"access [token] for private Github repos"`
	WebhookSecret string   `usage:"secret key coordinated with webhook declaration in Github"`
	MatchingRefs  []string `usage:"if given, limit to push events that regex-match"`
}

func (c *webhookServerCmd) Name() string {
	return "webhook-server"
}

func (c *webhookServerCmd) Synopsis() string {
	return "Run a web server to handle Github webhooks"
}

func (c *webhookServerCmd) Usage() string {
	return `webhook-server [options]
`
}

func (c *webhookServerCmd) SetFlags(f *flag.FlagSet) {
	filler := flagsfiller.New(flagsfiller.WithEnv(""))
	err := filler.Fill(f, c)
	if err != nil {
		log.Fatal(err)
	}
}

func (c *webhookServerCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	logger := args[0].(*zap.SugaredLogger)
	config := args[1].(*Config)

	logger.Debugw("running webhook-server")

	authenticator, err := OptionalIdentityAuthenticator(logger, config)
	if err != nil {
		logger.Errorw("failed to setup authenticator", "err", err)
		return subcommands.ExitFailure
	}

	loader, err := NewLoader(logger, authenticator, config.AdminUrl)

	gitContentBuilder := func(repository string, sha string) SourceContent {
		return NewSourceContentFromGit(logger, repository, sha, c.GithubToken)
	}

	webhookServer := NewWebhookServer(logger, loader, c.Port, gitContentBuilder, c.WebhookSecret, c.MatchingRefs)

	// blocks unless error at startup
	err = webhookServer.Start()
	if err != nil {
		logger.Errorw("webhook server failed", "err", err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}
