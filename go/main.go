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



func main() {
	currentUUID := uuid.NewV4()
	id := strings.Replace(currentUUID.String(), "-", "", -1)
	// privateZoneId := "privateZone_" + id
	privateZoneId := "dummy"
	resourceId := "resourceId_" + id
	tenantId := "aaaaaa"
	publicApiUrl := "http://localhost:8080/"
	agentReleaseUrl := publicApiUrl + "v1.0/tenant/" + tenantId + "/agent-releases"

	certDir := "/Users/geor7956/incoming/s4/salus-telemetry-bundle/dev/certs"
	message := `{"name": "` + privateZoneId + `"}`


	type LabelsType = struct {
		AgentDiscoveredArch string `json:"agent_discovered_arch"`
		AgentDiscoveredOs string `json:"agent_discovered_os"`
	}
	type agentReleaseEntry = struct {
		Id string
		ArType string `json:"type"`
		Version string
		Labels LabelsType
		Url string
		Exe string
	}
	type agentReleaseType = struct {
		Content []agentReleaseEntry
	}

	regularToken := ""
	req, err := http.NewRequest("GET", agentReleaseUrl, nil)
	if err != nil {
		log.Fatalln(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-auth-token", regularToken)

	// Do the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalln(err)
	}
//	body2 := []byte(`{"content":[{"id":"gbjid1","type":"TELEGRAF","version":"1.11.0","labels":{"agent_discovered_arch":"amd64","agent_discovered_os":"darwin"},"url":"https://homebrew.bintray.com/bottles/telegraf-1.11.0.high_sierra.bottle.tar.gz","exe":"telegraf/1.11.0/bin/telegraf","createdTimestamp":"2019-07-19T22:56:43Z","updatedTimestamp":"2019-07-19T22:56:43Z"},{"id":"7376d778-5f3f-43ee-8c21-2c79323481b0","type":"TELEGRAF","version":"1.11.0","labels":{"agent_discovered_arch":"amd64","agent_discovered_os":"linux"},"url":"https://dl.influxdata.com/telegraf/releases/telegraf-1.11.0-static_linux_amd64.tar.gz","exe":"./telegraf/telegraf","createdTimestamp":"2019-07-19T22:57:31Z","updatedTimestamp":"2019-07-19T22:57:31Z"}],"number":0,"totalPages":1,"totalElements":2,"last":true,"first":true}`)


	//defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	ar := new(agentReleaseType)
	err = json.Unmarshal(body, &ar)
	if err != nil {
		log.Fatalln(err)
	}

	localConfigTemplate := `resource_id: {{.ResourceId}}
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
	type TemplateFields = struct {
		ResourceId string
		PrivateZoneID string
		CertDir string
	}

	dir, err := ioutil.TempDir("", "e2et")
	if err != nil {
		log.Fatal(err)
	}

	configFileName := dir + "/config.yml"
	f, err := os.Create(configFileName)
	if err != nil {
		log.Fatal(err)
	}
	tmpl, err := template.New("t1").Parse(localConfigTemplate)
	if err != nil {
		log.Fatal(err)
	}
	tmpl.Execute(f, TemplateFields{resourceId, privateZoneId, certDir})

	envoyExeDir := "/Users/geor7956/go/bin/"

	cmd := exec.Command(envoyExeDir + "telemetry-envoy", "run", "--config=" + configFileName)
	cmd.Dir	= dir
	cmd.Stdout, err = os.Create(dir + "/envoyStdout")
	if err != nil {
		log.Fatal(err)
	}
	cmd.Stderr, err = os.Create(dir + "/envoyStderr")
	if err != nil {
		log.Fatal(err)
	}

	err = cmd.Start()
	if err != nil {
		log.Fatal(err)
	}

	req, err = http.NewRequest("POST", "http://localhost:8080/v1.0/tenant/" + tenantId + "/zones", bytes.NewBuffer([]byte(message)))
	if err != nil {
		log.Fatalln(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-auth-token", "application/json")

	// Do the request
	client = &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		log.Fatalln(err)
	}


	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	fmt.Println("gbj resp " + resp.Status + string(body))
	r := kafka.NewReader(kafka.ReaderConfig{
    Brokers:   []string{"localhost:9092"},
    Topic:     "salus.events.json",
    MinBytes:  1, // 10KB
    MaxBytes:  10e6, // 10MB
})

for {
    m, err := r.ReadMessage(context.Background())
    if err != nil {
        break
    }
    fmt.Printf("message at topic/partition/offset %v/%v/%v: %s = %s\n", m.Topic, m.Partition, m.Offset, string(m.Key), string(m.Value))
}

r.Close()
}
