/*
 *    Copyright 2018 Rackspace US, Inc.
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

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type LabelsType = struct {
	AgentDiscoveredArch string `json:"agent_discovered_arch"`
	AgentDiscoveredOs   string `json:"agent_discovered_os"`
}
type agentReleaseEntry = struct {
	Id      string
	ArType  string `json:"type"`
	Version string
	Labels  LabelsType
	Url     string
	Exe     string
}
type agentReleaseType = struct {
	Content []agentReleaseEntry
}

type TemplateFields = struct {
	ResourceId    string
	PrivateZoneID string
	CertDir       string
}

var localConfigTemplate = `resource_id: {{.ResourceId}}
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

type config = struct {
	currentUUID     uuid.UUID
	id              string
	privateZoneId   string
	resourceId      string
	tenantId        string
	publicApiUrl    string
	agentReleaseUrl string
	certDir         string
	regularToken    string
	dir             string
}

func initConfig() config {
	var c config
	c.currentUUID = uuid.NewV4()
	c.id = strings.Replace(c.currentUUID.String(), "-", "", -1)
	// privateZoneId = "privateZone_" + id
	c.privateZoneId = "dummy"
	c.resourceId = "resourceId_" + c.id
	c.tenantId = "aaaaaa"
	c.publicApiUrl = "http://localhost:8080/"
	c.agentReleaseUrl = c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/agent-releases"
	c.certDir = "/Users/geor7956/incoming/s4/salus-telemetry-bundle/dev/certs"
	c.regularToken = ""
	dir, err := ioutil.TempDir("", "e2et")
	checkErr(err, "error creating temp dir")
	c.dir = dir
	return c
}

func checkErr(err error, message string) {
	if err != nil {
		log.Fatal(message + ":" + err.Error())
	}
}

func main() {
	c := initConfig()
	ar := getReleases(c)
	fmt.Println("gbjcontent: " + ar.Content[0].Id)

	fmt.Println("gbjdir: " + c.dir)
	initEnvoy(c)

	message := `{"name": "` + c.privateZoneId + `"}`
	req, err := http.NewRequest("POST", "http://localhost:8080/v1.0/tenant/"+c.tenantId+"/zones", bytes.NewBuffer([]byte(message)))
	if err != nil {
		log.Fatalln(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-auth-token", "application/json")

	// Do the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalln(err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	fmt.Println("gbj resp " + resp.Status + string(body))
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{"localhost:9092"},
		Topic:    "salus.events.json",
		MinBytes: 1,    // 10KB
		MaxBytes: 10e6, // 10MB
	})

	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			break
		}
		fmt.Printf("message at topic/partition/offset %v/%v/%v: %s = %s\n", m.Topic, m.Partition, m.Offset, string(m.Key), string(m.Value))
		break
	}

	r.Close()
}

func initEnvoy(c config) {
	configFileName := c.dir + "/config.yml"
	f, err := os.Create(configFileName)
	if err != nil {
		log.Fatal(err)
	}
	tmpl, err := template.New("t1").Parse(localConfigTemplate)
	if err != nil {
		log.Fatal(err)
	}
	tmpl.Execute(f, TemplateFields{c.resourceId, c.privateZoneId, c.certDir})
	envoyExeDir := "/Users/geor7956/go/bin/"
	cmd := exec.Command(envoyExeDir+"telemetry-envoy", "run", "--config="+configFileName)
	cmd.Dir = c.dir
	cmd.Stdout, err = os.Create(c.dir + "/envoyStdout")
	if err != nil {
		log.Fatal(err)
	}
	cmd.Stderr, err = os.Create(c.dir + "/envoyStderr")
	if err != nil {
		log.Fatal(err)
	}
	err = cmd.Start()
	if err != nil {
		log.Fatal(err)
	}
}

func getReleases(c config) *agentReleaseType {
	req, err := http.NewRequest("GET", c.agentReleaseUrl, nil)
	if err != nil {
		log.Fatalln(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-auth-token", c.regularToken)
	// Do the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalln(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	ar := new(agentReleaseType)
	err = json.Unmarshal(body, ar)
	if err != nil {
		log.Fatalln(err)
	}
	return ar
}
