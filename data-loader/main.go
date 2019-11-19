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
	"fmt"
	"github.com/google/subcommands"
	"github.com/itzg/go-flagsfiller"
	"os"
)

type Config struct {
	IdentityUrl      string `default:"https://identity.api.rackspacecloud.com" usage:"The base URL of the Identity endpoint to use for authentication"`
	IdentityUsername string `usage:"username of a user in Identity that has access to the Salus Admin API"`
	IdentityPassword string `usage:"if apikey is not provided, the password for the given user"`
	IdentityApikey   string `usage:"if password is not provided, the apikey for the given user"`

	AdminUrl string `usage:"The base URL of the Salus Admin API endpoint to use"`

	Debug bool `usage:"Enables debug level logging"`
}

func main() {

	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(&loadFromGitCmd{}, "loading")
	subcommands.Register(&loadFromLocalDirCmd{}, "loading")

	var config Config

	err := flagsfiller.Parse(&config, flagsfiller.WithEnv(""))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "internal error: %s", err)
		os.Exit(3)
	}

	SetupLogger(config.Debug)
	defer CloseLogger()

	log := CreateLogger("main")

	os.Exit(int(subcommands.Execute(context.Background(), log, &config)))
}
