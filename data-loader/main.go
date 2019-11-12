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
	"github.com/alexflint/go-arg"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
)

var args struct {
	FromGitRepo  string
	FromGitSha   string
	FromLocalDir string

	GithubToken string `arg:"env:GITHUB_TOKEN"`

	IdentityUrl      string `default:"https://identity.api.rackspacecloud.com" arg:"env"`
	IdentityUsername string `arg:"env"`
	IdentityPassword string `arg:"env"`
	IdentityApiKey   string `arg:"env"`

	AdminUrl string `arg:"required,env"`

	Debug bool
}

func main() {

	argsParser := arg.MustParse(&args)

	SetupLogger(args.Debug)
	defer CloseLogger()

	var sourceContent SourceContent
	if args.FromLocalDir != "" {
		sourceContent = NewSourceContentFromDir(args.FromLocalDir)
	} else if args.FromGitRepo != "" {
		sourceContent = NewSourceContentFromGit(args.FromGitRepo, args.FromGitSha, args.GithubToken)
	} else {
		argsParser.WriteHelp(os.Stderr)
		logrus.Fatal("source content needs to be configured")
	}

	//noinspection GoNilness
	sourceContentPath, err := sourceContent.Prepare()
	if err != nil {
		logrus.WithError(err).Fatal("failed to prepare source content")
	}
	//noinspection GoNilness
	defer sourceContent.Cleanup()

	var clientAuth *IdentityAuthenticator
	if !strings.Contains(args.AdminUrl, "localhost") {
		clientAuth = &IdentityAuthenticator{
			IdentityUrl: args.IdentityUrl,
			Username:    args.IdentityUsername,
			Password:    args.IdentityPassword,
			Apikey:      args.IdentityApiKey,
		}
	}

	loader, err := NewLoader(clientAuth, args.AdminUrl, sourceContentPath)
	if err != nil {
		logrus.WithError(err).Fatal("failed to create loader")
	}

	err = loader.LoadAll()
	if err != nil {
		logrus.WithError(err).Fatal("failed to perform all loading")
	}
}
