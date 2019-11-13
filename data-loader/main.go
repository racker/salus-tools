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
	"github.com/iancoleman/strcase"
	"os"
	"strings"
)

type KebabLongSnakeEnvRenamer struct{}

func (KebabLongSnakeEnvRenamer) RenameLong(field string) string {
	return strcase.ToKebab(field)
}

func (KebabLongSnakeEnvRenamer) RenameEnv(field string) string {
	return strcase.ToScreamingSnake(field)
}

var args struct {
	KebabLongSnakeEnvRenamer

	FromGitRepo  string `help:"when given, enables data loading from a cloned git repo at the given URL"`
	FromGitSha   string `help:"when using a git repo, a specific commit SHA can be checked out"`
	FromLocalDir string `help:"for development, an existing directory can be referenced for data loading"`

	GithubToken string `arg:"env" help:"when using a private Github repo, an access token must be provided"`

	IdentityUrl      string `default:"https://identity.api.rackspacecloud.com" arg:"env" help:"The base URL of the Identity endpoint to use for authentication"`
	IdentityUsername string `arg:"env" help:"username of a user in Identity that has access to the Salus Admin API"`
	IdentityPassword string `arg:"env" help:"if apikey is not provided, the password for the given user"`
	IdentityApikey   string `arg:"env" help:"if password is not provided, the apikey for the given user"`

	AdminUrl string `arg:"required,env" help:"The base URL of the Salus Admin API endpoint to use"`

	Debug bool `help:"Enables debug level logging"`
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
		clientAuth, err = NewIdentityAuthenticator(log,
			args.IdentityUrl, args.IdentityUsername, args.IdentityPassword, args.IdentityApikey)
		if err != nil {
			log.Fatalw("failed to setup Identity authenticator", "err", err)
		}
	}

	loader, err := NewLoader(log, clientAuth, args.AdminUrl, sourceContentPath)
	if err != nil {
		log.Fatalw("failed to create loader", "err", err)
	}

	err = loader.LoadAll(sourceContentPath)
	if err != nil {
		log.Fatalw("failed to perform all loading", "err", err)
	}
}
