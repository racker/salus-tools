/*
 *    Copyright 2019 Rackspace US, Inc.
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 *
 *
 */

package main

const localConfigTemplate = `resource_id: {{.ResourceId}}
zone: {{.PrivateZoneID}}
labels:
  environment: localdev
tls:
  provided:
    ca: {{.CertDir}}/out/ca.pem
    cert: {{.CertDir}}/out/tenantA.pem
    key: {{.CertDir}}/out/tenantA-key.pem
ambassador:
  address: localhost:6565
ingest:
  lumberjack:
    bind: localhost:5044
  telegraf:
    json:
      bind: localhost:8094
agents:
  dataPath: data-telemetry-envoy
`
const remoteConfigTemplate = `resource_id: "{{.ResourceId }}"
zone: {{.PrivateZoneID}}
tls:
  auth_service:
    url: {{.AuthUrl}}
    token_provider: keystone_v2
  token_providers:
    keystone_v2:
      username: "{{.RegularId}}"
      apikey: "{{.ApiKey}}"
ambassador:
  address: {{.AmbassadorAddress}}
agents:
  dataPath: data-telemetry-envoy
`

const linuxReleaseData = `{
  "type": "TELEGRAF",
  "version": "1.11.0",
  "labels": {
    "agent_discovered_os": "linux",
    "agent_discovered_arch": "amd64"
  },
  "url": "https://dl.influxdata.com/telegraf/releases/telegraf-1.11.0-static_linux_amd64.tar.gz",
  "exe": "./telegraf/telegraf"
}
`
const darwinReleaseData = `{
  "type": "TELEGRAF",
  "version": "1.11.0",
  "labels": {
    "agent_discovered_os": "darwin",
    "agent_discovered_arch": "amd64"
  },
  "url": "https://homebrew.bintray.com/bottles/telegraf-1.11.0.high_sierra.bottle.tar.gz",
  "exe": "telegraf/1.11.0/bin/telegraf"
}`

const taskData = `{
	"name": "%s",
	"measurement": "%s",
	"taskParameters": {
		"labelSelector": {
			"agent_discovered_os": "%s"
		},
		"critical": {
			"consecutiveCount": 1,
			"expression": {
				"field": "result_code",
				"threshold": -1,
				"comparator": ">"
			}
		}
	}
}`

const netMonitorData = `{
  "labelSelector": {
    "agent_discovered_os": "%s"
  },
  "interval": "PT30S",
  "details": {
    "type": "remote",
    "monitoringZones": ["%s"],
    "plugin": {
      "type": "net_response",
      "address": "localhost:%s",
      "protocol": "tcp"
    }
  }
}`
const httpMonitorData = `{
  "labelSelector": {
    "agent_discovered_os": "%s"
  },
  "interval": "PT30S",
  "details": {
    "type": "remote",
    "monitoringZones": ["%s"],
    "plugin": {
      "type": "http_response",
      "address": "http://www.google.com:%s",
      "responseTimeout": "3s",
      "method": "GET"
    }
  }
}`

const monitorPolicyData = `{
  "scope": "TENANT",
  "subscope": "%s",
  "name": "E2ET_%s",
  "monitorId": "%s"
}`
