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
	"crypto/tls"
	"crypto/x509"
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
	"context"
	"github.com/spf13/viper"
	//"flag"
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
	ApiKey        string
	RegularId        string
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
var remoteConfigTemplate =
	`resource_id: "{{.ResourceId }}"
zone: {{.PrivateZoneID}}
tls:
  auth_service:
    url: https://salus-auth-serv.dev.monplat.rackspace.net
    token_provider: keystone_v2
  token_providers:
    keystone_v2:
      username: "{{.RegularId}}"
      apikey: "{{.ApiKey}}"
ambassador:
  address: salus-ambassador.dev.monplat.rackspace.net:443
agents:
  dataPath: data-telemetry-envoy
`
type config = struct {
	env string
	currentUUID     uuid.UUID
	id              string
	privateZoneId   string
	resourceId      string
	tenantId        string
	regularId       string
	publicApiUrl    string
	adminApiUrl		string
	agentReleaseUrl string
	certDir         string
	regularToken    string
	adminToken		string
	dir             string
	kafkaBrokers    []string
	topic           string
	port            string
	certFile string
	keyFile string
	caFile string
}

func initConfig() config {
	replacer := strings.NewReplacer(".", "_", "-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetEnvPrefix("E2ET")
	viper.AutomaticEnv() // read in environment variables that match

	//cfgFile := flag.String("cfgFile", "config.yml", "config file")
	cfgFile := "/Users/geor7956/incoming/s4/salus-telemetry-bundle/tools/go/config-local.yml"
	viper.SetConfigFile(cfgFile)
	if err := viper.ReadInConfig(); err == nil {
		log.Println("loaded: " + cfgFile)
	} else {
		log.Fatal("Config file not found" + cfgFile)
	}
	var c config
	c.env = viper.GetString("env")
	c.currentUUID = uuid.NewV4()
	c.id = strings.Replace(c.currentUUID.String(), "-", "", -1)
	c.privateZoneId = "privateZone_" + c.id
	c.resourceId = "resourceId_" + c.id
	c.tenantId = viper.GetString("tenantId")
	c.publicApiUrl = viper.GetString("publicApiUrl")
	c.adminApiUrl = viper.GetString("adminApiUrl")
	c.agentReleaseUrl = c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/agent-releases"
	c.certDir = viper.GetString("certDir")
	c.regularToken = "AADSH5trs1pHT2ZqqKHQfkeI2F2aMQFMCincDY0NB-nX0QM7qjeSOQFKMEghGdnLgBnJAH_5D531J4eJGiFZaeGNHa1oWEqRLwFQe8Q3PDN-d8nQAGohh_JYRIMonzZC4FDIO4IoYVEe4g"
	c.adminToken = "AADSH5tr4v_5_p-cz_SsUYCr7hreLKMfQvbfPEwtAx2DpEMgxg1MSA8dwewUrrLuSIg7oZF49RPk-EebBOFEmbAC7y6zbTJ-lN55gxMY2ArKtHTuTJlDroS9"
	c.regularId = viper.GetString("regularId")
	dir, err := ioutil.TempDir("", "e2et")
	checkErr(err, "error creating temp dir")
	c.dir = dir
	//_ = viper.UnmarshalKey("kafkaBrokers", c.kafkaBrokers)
	c.kafkaBrokers = viper.GetStringSlice("kafkaBrokers")
	c.topic = viper.GetString("topic")
	//gbj pick port dynamically
	c.port = "8222"
	c.certFile = viper.GetString("certFile")
	c.keyFile = viper.GetString("keyFile")
	c.caFile = viper.GetString("caFile")
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
	eventFound := make(chan bool, 1)
	go checkForEvents(c, eventFound)
	createMonitor(c)
	<-eventFound
}

func initEnvoy(c config, releaseId string) {
	log.Println("starting envoy")
	configFileName := c.dir + "/config.yml"
	f, err := os.Create(configFileName)
	if err != nil {
		log.Fatal(err)
	}
	var configTemplate string
	if (c.env == "local") {
		configTemplate = localConfigTemplate
	} else {
		configTemplate = remoteConfigTemplate
	}
	tmpl, err := template.New("t1").Parse(configTemplate)
	if err != nil {
		log.Fatal(err)
	}

	tmpl.Execute(f, TemplateFields{c.resourceId, c.privateZoneId,
		c.certDir, os.Getenv("GBJ_API_KEY"), c.regularId})
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

	log.Println("gbj create install %s %s", url, fmt.Sprintf(installData, releaseId, runtime.GOOS))
	_ = doReq("POST", url, fmt.Sprintf(installData, releaseId, runtime.GOOS), "creating agent install", c.regularToken)
	// give it time to install
	time.Sleep(10 * time.Second)
	if _, err = os.Stat(c.dir + "/data-telemetry-envoy"); err != nil {
		log.Fatal("install failed")
	}
	log.Println("envoy started")
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
	log.Println("gbj get: %s %s", c.agentReleaseUrl, c.regularToken)
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
	if entry.Version == "0.0.0" {
		releaseBody, ok := releaseData[runtime.GOOS + "-" + runtime.GOARCH]
		if !ok {
			log.Fatal("no valid release found for this arch")
		}
		newArBody := doReq("POST",  c.adminApiUrl + "api/agent-releases",
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
	log.Println("deleting AgentInstalls")
		url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/agent-installs/"
		installBody := doReq("GET", url,
		"", "getting all agent installs", c.regularToken)
		var resp GetAgentInstallsResp
		err := json.Unmarshal(installBody, &resp)
		checkErr(err, "unable to parse get agent installs response")
		for _, i := range resp.Content {
			// delete each install
			log.Println("gbj deleting: " + url +i.ID)
			_ = doReq("DELETE", url + i.ID, "", "deleting agent install " + i.ID, c.regularToken)

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
	log.Println("deleting Resources")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/resources/"
	body := doReq("GET", url,
		"", "getting all resources", c.regularToken)
	var resp GetResourcesResp
	err := json.Unmarshal(body, &resp)
	checkErr(err, "unable to parse get resources response")
	for _, i := range resp.Content {
		log.Println("delete resource: " + i.ResourceID)
		// delete each resource
		_ = doReq("DELETE", url + i.ResourceID, "", "deleting resource " + i.ResourceID, c.regularToken)

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
	log.Println("deleting Monitors")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/monitors/"
	for {
		body := doReq("GET", url,
			"", "getting all monitors", c.regularToken)
		var resp GetMonitorsResp
		err := json.Unmarshal(body, &resp)
		checkErr(err, "unable to parse get monitors response")
		for _, i := range resp.Content {
			// delete each monitor
			_ = doReq("DELETE", url + i.ID, "", "deleting monitor " + i.ID, c.regularToken)

		}
		if resp.Last {
			break
		}
	}
}

func createPrivateZone(c config) {
	log.Println("deleting private zones")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/zones/"
	for {
		body := doReq("GET", url,
			"", "getting all zones", c.regularToken)
		var resp GetZonesResp
		err := json.Unmarshal(body, &resp)
		checkErr(err, "unable to parse get zones response")
		for _, i := range resp.Content {
			// delete each zone
			if strings.Index(i.Name, "public/") != 0 {
				_ = doReq("DELETE", url + i.Name, "", "deleting zone " + i.Name, c.regularToken)
			}
		}
		if resp.Last {
			break
		}
	}

	// Now create new one
	message := `{"name": "` + c.privateZoneId + `"}`
	log.Println("creating zone: %s %s", url, message)
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
	log.Println("deleting Tasks")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/event-tasks/"
	for {
		body := doReq("GET", url,
			"", "getting all tasks", c.regularToken)
		var resp GetTasksResp
		err := json.Unmarshal(body, &resp)
		checkErr(err, "unable to parse get tasks response")
		for _, i := range resp.Content {
			// delete each task
			_ = doReq("DELETE", url + i.ID, "", "deleting tasks " + i.ID, c.regularToken)

		}
		if resp.Last {
			break
		}
	}

	// Now create new one
	data := fmt.Sprintf(taskData, "task_" + c.id, runtime.GOOS)
	_ = doReq("POST", url, data, "creating task", c.regularToken)
	
}

func checkForEvents(c config, eventFound chan bool) {
	var r *kafka.Reader
	if (c.env == "local") {

		r = kafka.NewReader(kafka.ReaderConfig{
			Brokers:  c.kafkaBrokers,
			Topic:    c.topic,
			MinBytes: 1,    // 10KB
			MaxBytes: 10e6, // 10MB
		})

	} else {
		// Load client cert
		cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
		if err != nil {
			log.Fatal(err)
		}

		// Load CA cert
		caCert, err := ioutil.ReadFile(c.caFile)
		if err != nil {
			log.Fatal(err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		// Setup HTTPS client
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
		}
		tlsConfig.BuildNameToCertificate()
		dialer := &kafka.Dialer{
			Timeout:   10 * time.Second,
			DualStack: true,
			TLS:       tlsConfig,
		}

		r = kafka.NewReader(kafka.ReaderConfig{
			Brokers:  c.kafkaBrokers,
			Topic:    c.topic,
			MinBytes: 1,    // 10KB
			MaxBytes: 10e6, // 10MB
			Dialer:   dialer,
		})
	}
	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			break
		}
		log.Printf("message at topic/partition/offset %v/%v/%v: %s = %s\n", m.Topic, m.Partition, m.Offset, string(m.Key), string(m.Value))
		s := string(m.Value)
		if strings.Contains(s, c.resourceId) {
			eventFound <- true
		}
	}

	r.Close()
	
}

var monitorData =
	`{
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
func createMonitor(c config ) {
	log.Println("creating Monitor")
	
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/monitors/"
	data := fmt.Sprintf(monitorData, runtime.GOOS, c.privateZoneId, c.port)
	_ = doReq("POST", url, data, "creating monitor", c.regularToken)
	log.Println("monitor created")

}