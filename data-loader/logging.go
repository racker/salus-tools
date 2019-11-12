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
	"go.uber.org/zap"
	"log"
	"reflect"
	"strings"
	"sync"
)

var logger *zap.Logger
var loggerSetup sync.Once

func SetupLogger(debug bool) {
	loggerSetup.Do(func() {
		var err error
		if debug {
			logger, err = zap.NewDevelopment()
		} else {
			logger, err = zap.NewProduction()
		}
		if err != nil {
			log.Fatalf("unable to setup zap logger: %v", err)
		}
	})
}

func CloseLogger() {
	_ = logger.Sync()
}

func CreateLogger(scope interface{}) *zap.SugaredLogger {
	if scope == nil {
		return logger.Sugar()
	} else if name, ok := scope.(string); ok {
		return logger.Named(name).Sugar()
	} else {
		val := reflect.ValueOf(scope)
		t := val.Type()

		parts := make([]string, 0, 2)
		if t.PkgPath() != "" {
			parts = append(parts, t.PkgPath())
		}
		if t.Name() != "" {
			parts = append(parts, t.Name())
		}

		name := strings.Join(parts, ".")
		return logger.Named(name).Sugar()
	}
}
