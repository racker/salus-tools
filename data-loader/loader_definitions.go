/*
 * Copyright 2020 Rackspace US, Inc.
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

var loaderDefinitions = []LoaderDefinition{
	{
		Name:    "agent-releases",
		ApiPath: "/api/agent-releases",
		UniqueFieldPaths: []string{
			"$.type",
			"$.version",
			"$.labels.agent_discovered_os",
			"$.labels.agent_discovered_arch",
		},
	},
	{
		Name:    "monitor-translations",
		ApiPath: "/api/monitor-translations",
		UniqueFieldPaths: []string{
			"$.monitorType",
			"$.name",
		},
	},
	{
		Name:    "policy-monitors",
		ApiPath: "/api/policy-monitors",
		UniqueFieldPaths: []string{
			"$.name",
		},
	},
	{
		Name:    "monitor-metadata-policies",
		ApiPath: "/api/policy/metadata/monitor",
		UniqueFieldPaths: []string{
			"$.scope",
			"$.subscope",
			"$.targetClassName",
			"$.valueType",
			"$.key",
		},
	},
}
