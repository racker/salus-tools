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

	log := CreateLogger("main")

	var sourceContent SourceContent
	if args.FromLocalDir != "" {
		sourceContent = NewSourceContentFromDir(log, args.FromLocalDir)
	} else if args.FromGitRepo != "" {
		sourceContent = NewSourceContentFromGit(log, args.FromGitRepo, args.FromGitSha, args.GithubToken)
	} else {
		argsParser.WriteHelp(os.Stderr)
		log.Fatal("source content needs to be configured")
	}

	//noinspection GoNilness
	sourceContentPath, err := sourceContent.Prepare()
	if err != nil {
		log.Fatalw("failed to prepare source content", "err", err)
	}
	//noinspection GoNilness
	defer sourceContent.Cleanup()

	var clientAuth *IdentityAuthenticator
	if !strings.Contains(args.AdminUrl, "localhost") {
		clientAuth = NewIdentityAuthenticator(log,
			args.IdentityUrl, args.IdentityUsername, args.IdentityPassword, args.IdentityApiKey)
	}

	loader, err := NewLoader(log, clientAuth, args.AdminUrl, sourceContentPath)
	if err != nil {
		log.Fatalw("failed to create loader", "err", err)
	}

	err = loader.LoadAll()
	if err != nil {
		log.Fatalw("failed to perform all loading", "err", err)
	}
}
