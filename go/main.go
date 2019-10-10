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
	"github.com/coreos/go-semver/semver"
	"github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"runtime"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
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
	adminApiUrl		string
	agentReleaseUrl string
	certDir         string
	regularToken    string
	adminToken		string
	dir             string
}

func initConfig() config {
	var c config
	c.currentUUID = uuid.NewV4()
	c.id = strings.Replace(c.currentUUID.String(), "-", "", -1)
	c.privateZoneId = "privateZone_" + c.id
	//c.privateZoneId = "dummy"
	c.resourceId = "resourceId_" + c.id
	c.tenantId = "aaaaaa"
	c.publicApiUrl = "http://localhost:8080/"
	c.adminApiUrl = "http://localhost:8888"
	c.agentReleaseUrl = c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/agent-releases"
	c.certDir = "/Users/geor7956/incoming/s4/salus-telemetry-bundle/dev/certs"
	c.regularToken = ""
	c.adminApiUrl = ""
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
	fmt.Println("gbjdir: " + c.dir)

	releaseId := getReleases(c)
	fmt.Println("gbjr: " + releaseId)
	deleteAgentInstalls(c)
	deleteResources(c)
	deleteMonitors(c)
	createPrivateZone(c)
	
	initEnvoy(c, releaseId)
	createTask(c)

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

func initEnvoy(c config, releaseId string) {
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

	// give it time to start
	time.Sleep(10 * time.Second)
	if _, err = os.Stat(c.dir + "/data-telemetry-envoy"); err == nil {
		log.Fatal("installs incorrectly removed")
	}
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/agent-installs"
	installData := `{
	"agentReleaseId": "%s",
	"labelSelector": {
		"agent_discovered_os": "%s"
	}}`
	_ = doReq("POST", url, fmt.Sprintf(installData, releaseId, runtime.GOOS), "creating agent install", c.regularToken)
	// give it time to install
	time.Sleep(10 * time.Second)
	if _, err = os.Stat(c.dir + "/data-telemetry-envoy"); err != nil {
		log.Fatal("install failed")
	}
	
}
var linuxReleaseData =
	`{
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
	var darwinReleaseData =
	`{
  "type": "TELEGRAF",
  "version": "1.11.0",
  "labels": {
    "agent_discovered_os": "darwin",
    "agent_discovered_arch": "amd64"
  },
  "url": "https://homebrew.bintray.com/bottles/telegraf-1.11.0.high_sierra.bottle.tar.gz",
  "exe": "telegraf/1.11.0/bin/telegraf"
}`
type AgentReleaseCreateResp struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Version string `json:"version"`
	Labels  struct {
		AgentDiscoveredOs   string `json:"agent_discovered_os"`
		AgentDiscoveredArch string `json:"agent_discovered_arch"`
	} `json:"labels"`
	URL              string    `json:"url"`
	Exe              string    `json:"exe"`
	CreatedTimestamp time.Time `json:"createdTimestamp"`
	UpdatedTimestamp time.Time `json:"updatedTimestamp"`
}

func getReleases(c config) string {
	releaseData := make(map[string]string)
	releaseData["linux-amd64"] = linuxReleaseData
	releaseData["darwin-amd64"] = darwinReleaseData

	var ar agentReleaseType
	arBody := doReq("GET", c.agentReleaseUrl, "", "getting all agent releases",
		c.regularToken)
	err := json.Unmarshal(arBody, &ar)
	checkErr(err, "unable to parse agent release response")
	// get the latest matching release
	var entry agentReleaseEntry
	entry.Version = "0.0.0"
	for _, r := range ar.Content {
		if r.Labels.AgentDiscoveredArch == runtime.GOARCH &&
			r.Labels.AgentDiscoveredOs == runtime.GOOS {
			if semver.New(entry.Version).LessThan(*semver.New(r.Version)) {
				entry = r
			}
		}
	}
	//create release if none exists
	if entry.Version == "" {
		releaseBody, ok := releaseData[runtime.GOOS + "-" + runtime.GOARCH]
		if !ok {
			log.Fatal("no valid release found for this arch")
		}
		newArBody := doReq("POST",  c.adminApiUrl + "/api/agent-releases",
			releaseBody, "creating new agent release", c.adminToken)

		createResp := new(AgentReleaseCreateResp)
		err = json.Unmarshal(newArBody, createResp)
		checkErr(err, "unable to parse create response")
		return createResp.ID
		} else {
			return entry.Id
	}
	
}

func doReq(method string, url string, data string, errMessage string, token string) ([]byte){
		req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(data)))
		checkErr(err, "request create failed: " + errMessage)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-auth-token", token)
		// Do the request
		client := &http.Client{}
		resp, err := client.Do(req)
		checkErr(err, "client create failed: " + errMessage)
		if resp.StatusCode/100 != 2 {
			log.Fatal(errMessage + ": status code: " + resp.Status)
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		checkErr(err, "unable read response body: " + errMessage)
		return body
}
type GetAgentInstallsResp struct {
	Content []struct {
		ID           string `json:"id"`
		AgentRelease struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Version string `json:"version"`
			Labels  struct {
				AgentDiscoveredArch string `json:"agent_discovered_arch"`
				AgentDiscoveredOs   string `json:"agent_discovered_os"`
			} `json:"labels"`
			URL              string    `json:"url"`
			Exe              string    `json:"exe"`
			CreatedTimestamp time.Time `json:"createdTimestamp"`
			UpdatedTimestamp time.Time `json:"updatedTimestamp"`
		} `json:"agentRelease"`
		LabelSelector struct {
			AgentDiscoveredOs string `json:"agent_discovered_os"`
		} `json:"labelSelector"`
		CreatedTimestamp time.Time `json:"createdTimestamp"`
		UpdatedTimestamp time.Time `json:"updatedTimestamp"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}
	func deleteAgentInstalls(c config) {
		url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/agent-installs/"
		installBody := doReq("GET", url,
		"", "getting all agent installs", c.regularToken)
		var resp GetAgentInstallsResp
		err := json.Unmarshal(installBody, &resp)
		checkErr(err, "unable to parse get agent installs response")
		for _, i := range resp.Content {
			// delete each install
			_ = doReq("DELETE", url + i.ID, "", "deleting agent install" + i.ID, c.regularToken)

		}
	}

type GetResourcesResp struct {
	Content []struct {
		TenantID   string `json:"tenantId"`
		ResourceID string `json:"resourceId"`
		Labels     struct {
			AgentDiscoveredArch     string `json:"agent_discovered_arch"`
			AgentDiscoveredHostname string `json:"agent_discovered_hostname"`
			AgentEnvironment        string `json:"agent_environment"`
			AgentDiscoveredOs       string `json:"agent_discovered_os"`
		} `json:"labels"`
		Metadata                  interface{} `json:"metadata"`
		PresenceMonitoringEnabled bool        `json:"presenceMonitoringEnabled"`
		AssociatedWithEnvoy       bool        `json:"associatedWithEnvoy"`
		CreatedTimestamp          time.Time   `json:"createdTimestamp"`
		UpdatedTimestamp          time.Time   `json:"updatedTimestamp"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}
func deleteResources(c config) {
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/resources/"
	body := doReq("GET", url,
		"", "getting all resources", c.regularToken)
	var resp GetResourcesResp
	err := json.Unmarshal(body, &resp)
	checkErr(err, "unable to parse get resources response")
	for _, i := range resp.Content {
		// delete each resource
		_ = doReq("DELETE", url + i.ResourceID, "", "deleting resource" + i.ResourceID, c.regularToken)

	}
}


type GetZonesResp struct {
	Content []struct {
		Name              string        `json:"name"`
		PollerTimeout     int           `json:"pollerTimeout"`
		Provider          interface{}   `json:"provider"`
		ProviderRegion    interface{}   `json:"providerRegion"`
		SourceIPAddresses []interface{} `json:"sourceIpAddresses"`
		CreatedTimestamp  time.Time     `json:"createdTimestamp"`
		UpdatedTimestamp  time.Time     `json:"updatedTimestamp"`
		Public            bool          `json:"public"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}
type GetMonitorsResp struct {
	Content []struct {
		ID            string      `json:"id"`
		Name          interface{} `json:"name"`
		LabelSelector struct {
			AgentDiscoveredOs string `json:"agent_discovered_os"`
		} `json:"labelSelector"`
		LabelSelectorMethod string      `json:"labelSelectorMethod"`
		ResourceID          interface{} `json:"resourceId"`
		Interval            string      `json:"interval"`
		CreatedTimestamp time.Time `json:"createdTimestamp"`
		UpdatedTimestamp time.Time `json:"updatedTimestamp"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}

func deleteMonitors(c config) {
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/monitors/"
	for {
		body := doReq("GET", url,
			"", "getting all monitors", c.regularToken)
		var resp GetMonitorsResp
		err := json.Unmarshal(body, &resp)
		checkErr(err, "unable to parse get monitors response")
		for _, i := range resp.Content {
			// delete each monitor
			_ = doReq("DELETE", url + i.ID, "", "deleting zone" + i.ID, c.regularToken)

		}
		if resp.Last {
			break
		}
	}
}

func createPrivateZone(c config) {
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/zones/"
	for {
		body := doReq("GET", url,
			"", "getting all zones", c.regularToken)
		var resp GetZonesResp
		err := json.Unmarshal(body, &resp)
		checkErr(err, "unable to parse get zones response")
		for _, i := range resp.Content {
			// delete each zone
			_ = doReq("DELETE", url + i.Name, "", "deleting zone" + i.Name, c.regularToken)

		}
		if resp.Last {
			break
		}
	}

	// Now create new one
	message := `{"name": "` + c.privateZoneId + `"}`
	_ = doReq("POST", url, message, "creating private zone", c.regularToken)
	
}

var taskData =
	`{
	"name": "%s",
	"measurement": "net_response",
	"taskParameters": {
		"labelSelector": {
			"agent_discovered_os": "%s"
		},
		"critical": {
			"consecutiveCount": 1,
			"expression": {
				"field": "result_code",
				"threshold": 0.0,
				"comparator": ">"
			}
		}
	}
}`

type GetTasksResp struct {
	Content []struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Measurement    string `json:"measurement"`
		TaskParameters struct {
			Info     interface{} `json:"info"`
			Warning  interface{} `json:"warning"`
			Critical struct {
				Expression struct {
					Field      string `json:"field"`
					Threshold  int    `json:"threshold"`
					Comparator string `json:"comparator"`
				} `json:"expression"`
				ConsecutiveCount int `json:"consecutiveCount"`
			} `json:"critical"`
			EvalExpressions   interface{} `json:"evalExpressions"`
			WindowLength      interface{} `json:"windowLength"`
			WindowFields      interface{} `json:"windowFields"`
			FlappingDetection bool        `json:"flappingDetection"`
			LabelSelector     struct {
				AgentEnvironment string `json:"agent_environment"`
			} `json:"labelSelector"`
		} `json:"taskParameters"`
		CreatedTimestamp time.Time `json:"createdTimestamp"`
		UpdatedTimestamp time.Time `json:"updatedTimestamp"`
	} `json:"content"`
	Number        int  `json:"number"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	Last          bool `json:"last"`
	First         bool `json:"first"`
}
func createTask(c config) {
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/event-tasks/"
	for {
		body := doReq("GET", url,
			"", "getting all tasks", c.regularToken)
		var resp GetTasksResp
		err := json.Unmarshal(body, &resp)
		checkErr(err, "unable to parse get tasks response")
		for _, i := range resp.Content {
			// delete each task
			_ = doReq("DELETE", url + i.ID, "", "deleting tasks" + i.ID, c.regularToken)

		}
		if resp.Last {
			break
		}
	}

	// Now create new one
	data := fmt.Sprintf(taskData, "task_" + c.id, runtime.GOOS)
	_ = doReq("POST", url, data, "creating private zone", c.regularToken)
	
}
