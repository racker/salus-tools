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

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/coreos/go-semver/semver"
	"github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/spf13/viper"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

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

func initConfig() config {
	replacer := strings.NewReplacer(".", "_", "-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetEnvPrefix("E2ET")
	viper.AutomaticEnv() // read in environment variables that match

	cfgFile := flag.String("config", "config.yml", "config file")
	flag.Parse()
	viper.SetConfigFile(*cfgFile)
	if err := viper.ReadInConfig(); err == nil {
		log.Println("loaded: " + *cfgFile)
	} else {
		log.Fatal("Config file not found " + *cfgFile)
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
	c.regularId = viper.GetString("regularId")
	c.adminId = viper.GetString("adminId")
	dir, err := ioutil.TempDir("", "e2et")
	checkErr(err, "error creating temp dir")
	c.dir = dir
	c.kafkaBrokers = viper.GetStringSlice("kafkaBrokers")
	c.topic = viper.GetString("topic")
	c.identityUrl = viper.GetString("identityUrl")
	c.authUrl = viper.GetString("authUrl")
	c.ambassadorAddress = viper.GetString("ambassadorAddress")
	//gbj pick port dynamically?
	c.port = "8222"
	c.publicZoneId = "public/us_central_1"
	c.certFile = viper.GetString("certFile")
	c.keyFile = viper.GetString("keyFile")
	c.caFile = viper.GetString("caFile")
	c.regularApiKey = os.Getenv("E2ET_REGULAR_API_KEY")
	c.adminApiKey = os.Getenv("E2ET_ADMIN_API_KEY")
	c.adminPassword = os.Getenv("E2ET_ADMIN_PASSWORD")
	c.regularToken = getToken(c, c.regularId, c.regularApiKey, "")
	c.adminToken = getToken(c, c.adminId, c.adminApiKey, c.adminPassword)
	return c
}

const apiTokenData = `{
    "auth": {
       "RAX-KSKEY:apiKeyCredentials": {  
          "username": "%s",  
          "apiKey": "%s"}
    }  
}`
const pwTokenData = `{
   "auth": {
       "passwordCredentials": {
          "username":"%s",
          "password":"%s"
       }
    }
}`

func getToken(c config, user string, apiKey string, pw string) string {
	if user == "" {
		return ""
	}
	url := c.identityUrl
	var tokenData string
	if pw != "" {
		tokenData = fmt.Sprintf(pwTokenData, user, pw)
	} else {
		tokenData = fmt.Sprintf(apiTokenData, user, apiKey)
	}
	var resp IdentityResp
	body := doReq("POST", url, tokenData, "getting token for : "+user, "")
	err := json.Unmarshal(body, &resp)
	checkErr(err, "unable to parse identity response")
	return resp.Access.Token.ID
}
func checkErr(err error, message string) {
	if err != nil {
		log.Fatal(message + ":" + err.Error())
	}
}

func main() {
	log.Println("Starting e2et")
	c := initConfig()
	fmt.Println("Temp dir is : " + c.dir)

	releaseId := getReleases(c)
	deleteAgentInstalls(c)
	deleteResources(c)
	deleteMonitors(c)
	createPrivateZone(c)

	cmd := initEnvoy(c, releaseId)
	defer cmd.Process.Kill()
	createTask(c)
	eventFound := make(chan bool, 1)
	go checkForEvents(c, eventFound)
	createMonitor(c)
	createPolicyMonitor(c)
	checkPresenceMonitor(c)
	log.Println("looking for events:")
	select {
	case <-eventFound:
		log.Println("events returned from kafka successfully")
	case <-time.After(10 * time.Minute):
		log.Fatal("Timed out waiting for events")
	}

}

func initEnvoy(c config, releaseId string) (cmd *exec.Cmd) {
	log.Println("starting envoy")
	configFileName := c.dir + "/config.yml"
	f, err := os.Create(configFileName)
	if err != nil {
		log.Fatal(err)
	}
	var configTemplate string
	if c.env == "local" {
		configTemplate = localConfigTemplate
	} else {
		configTemplate = remoteConfigTemplate
	}
	tmpl, err := template.New("t1").Parse(configTemplate)
	if err != nil {
		log.Fatal(err)
	}

	tmpl.Execute(f, TemplateFields{c.resourceId, c.privateZoneId,
		c.certDir, c.regularApiKey, c.regularId, c.authUrl, c.ambassadorAddress})
	cmd = exec.Command(os.Getenv("GOPATH")+"/bin/telemetry-envoy", "run", "--config="+configFileName)
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
        "labelSelectorMethod": "AND",
	"labelSelector": {
		"agent_discovered_os": "%s"
	}}`

	_ = doReq("POST", url, fmt.Sprintf(installData, releaseId, runtime.GOOS), "creating agent install", c.regularToken)
	// give it time to install
	time.Sleep(20 * time.Second)
	if _, err = os.Stat(c.dir + "/data-telemetry-envoy"); err != nil {
		log.Fatal("install failed")
	}
	log.Println("envoy started")
	return cmd
}

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

func getReleases(c config) string {
	releaseData := make(map[string]string)
	releaseData["linux-amd64"] = linuxReleaseData
	releaseData["darwin-amd64"] = darwinReleaseData

	var ar AgentReleaseType
	arBody := doReq("GET", c.agentReleaseUrl, "", "getting all agent releases",
		c.regularToken)
	err := json.Unmarshal(arBody, &ar)
	checkErr(err, "unable to parse agent release response")
	// get the latest matching release
	var entry AgentReleaseEntry
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
		releaseBody, ok := releaseData[runtime.GOOS+"-"+runtime.GOARCH]
		if !ok {
			log.Fatal("no valid release found for this arch")
		}
		newArBody := doReq("POST", c.adminApiUrl+"api/agent-releases",
			releaseBody, "creating new agent release", c.adminToken)

		createResp := new(AgentReleaseCreateResp)
		err = json.Unmarshal(newArBody, createResp)
		checkErr(err, "unable to parse create response")
		return createResp.ID
	} else {
		return entry.Id
	}

}

const curlOutput = `
curl %s  \
    -X %s \
    -d '%s' \
    -H "Content-type: application/json" \
    -H "x-auth-token: %s"
`

func doReq(method string, url string, data string, errMessage string, token string) []byte {
	var printedToken string
	if os.Getenv("E2ET_PRINT_TOKENS") != "" {
		printedToken = token
	} else {
		printedToken = "xxx"
	}
	if !strings.Contains(data, "username") {
		log.Printf("Running equivalent curl:\n %s",
			fmt.Sprintf(curlOutput, url, method, strings.Replace(data, "\n", "", -1), printedToken))

	}
	req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(data)))
	checkErr(err, "request create failed: "+errMessage)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-auth-token", token)
	// Do the request
	client := &http.Client{}
	resp, err := client.Do(req)
	checkErr(err, "client create failed: "+errMessage)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	checkErr(err, "unable read response body: "+errMessage)
	if resp.StatusCode/100 != 2 {
		log.Println(errMessage + ": " + string(body))
		log.Fatal("status code: " + resp.Status)
	}

	return body
}

func deleteAgentInstalls(c config) {
	log.Println("deleting AgentInstalls")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/agent-installs/"
	for {
		installBody := doReq("GET", url,
			"", "getting all agent installs", c.regularToken)
		var resp GetAgentInstallsResp
		err := json.Unmarshal(installBody, &resp)
		checkErr(err, "unable to parse get agent installs response")
		for _, i := range resp.Content {
			// delete each install
			_ = doReq("DELETE", url+i.ID, "", "deleting agent install "+i.ID, c.regularToken)

		}
		if resp.Last {
			break
		}
	}

}

func deleteResources(c config) {
	log.Println("deleting Resources")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/resources/"
	for {
		body := doReq("GET", url,
			"", "getting all resources", c.regularToken)
		var resp GetResourcesResp
		err := json.Unmarshal(body, &resp)
		checkErr(err, "unable to parse get resources response")
		for _, i := range resp.Content {
			log.Println("delete resource: " + i.ResourceID)
			// delete each resource
			_ = doReq("DELETE", url+i.ResourceID, "", "deleting resource "+i.ResourceID, c.regularToken)

		}
		if resp.Last {
			break
		}
	}

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
			_ = doReq("DELETE", url+i.ID, "", "deleting monitor "+i.ID, c.regularToken)

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
				_ = doReq("DELETE", url+i.Name, "", "deleting zone "+i.Name, c.regularToken)
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
			_ = doReq("DELETE", url+i.ID, "", "deleting tasks "+i.ID, c.regularToken)

		}
		if resp.Last {
			break
		}
	}

	// Now create new one
	data := fmt.Sprintf(taskData, "net_response_task_"+c.id, "net_response", runtime.GOOS)
	_ = doReq("POST", url, data, "creating net task", c.regularToken)
	data = fmt.Sprintf(taskData, "http_response_task_"+c.id, "http_response", runtime.GOOS)
	_ = doReq("POST", url, data, "creating http task", c.regularToken)

}

func checkForEvents(c config, eventFound chan bool) {
	var r *kafka.Reader
	finishedMap := make(map[string]bool)
	finishedMap["net"] = false
	if c.env == "local" {
		r = kafka.NewReader(kafka.ReaderConfig{
			Brokers:  c.kafkaBrokers,
			Topic:    c.topic,
			MinBytes: 1,
			MaxBytes: 10e6, // 10MB
		})

	} else {
		// gbj finishedMap["http"] = false
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
			MinBytes: 1,
			MaxBytes: 10e6, // 10MB
			Dialer:   dialer,
		})
	}
	log.Println("waiting for events")
	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			break
		}
		log.Printf("message at topic/partition/offset %v/%v/%v: %s = %s\n", m.Topic, m.Partition, m.Offset, string(m.Key), string(m.Value))
		s := string(m.Value)
		if strings.Contains(s, c.resourceId) {
			if strings.Contains(s, "net_response") {
				finishedMap["net"] = true
			}
			if strings.Contains(s, "http_response") {
				finishedMap["http"] = true
			}
			var allFinished bool
			for _, b := range finishedMap {
				allFinished = b
				if !allFinished {
					break
				}
			}

			if allFinished {
				<-eventFound
			}
		}
	}

	r.Close()

}

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

func createMonitor(c config) {
	log.Println("creating Monitors")

	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/monitors/"
	data := fmt.Sprintf(netMonitorData, runtime.GOOS, c.privateZoneId, c.port)
	_ = doReq("POST", url, data, "creating net monitor", c.regularToken)

	if c.env != "local" {
		adminUrl := c.adminApiUrl + "api/policy-monitors"
		data = fmt.Sprintf(httpMonitorData, runtime.GOOS, c.publicZoneId, c.port)
		_ = doReq("POST", adminUrl, data, "creating http monitor", c.adminToken)
	}
	log.Println("monitors created")

}

const monitorPolicyData = `{
  "scope": "TENANT",
  "subscope": "%s",
  "name": "E2ET_%s",
  "monitorId": "%s"
}`

func createPolicyMonitor(c config) {
	// policy monitors require public pollers which local envs don't have
	if c.env == "local" {
		return
	}
	log.Println("deleting policy monitors")
	policyUrl := c.adminApiUrl + "api/policies/monitors/"
	monitorUrl := c.adminApiUrl + "api/policy-monitors/"

	for {
		body := doReq("GET", policyUrl,
			"", "getting all policy monitors", c.adminToken)
		var resp GetPoliciesResp
		err := json.Unmarshal(body, &resp)
		checkErr(err, "unable to parse get policy monitor response")
		for _, i := range resp.Content {
			// delete each policy
			_ = doReq("DELETE", policyUrl+i.ID, "", "deleting policy "+i.ID, c.adminToken)
			// delete the corresponding monitor
			_ = doReq("DELETE", monitorUrl+i.MonitorID, "", "deleting policy monitor "+i.MonitorID, c.adminToken)
		}
		if resp.Last {
			break
		}
	}

	// Now create new policy monitor
	data := fmt.Sprintf(httpMonitorData, runtime.GOOS, c.publicZoneId, c.port)
	body := doReq("POST", monitorUrl, data, "creating policy monitor", c.adminToken)
	var resp CreatePolicyMonitorResp
	err := json.Unmarshal(body, &resp)
	checkErr(err, "createing policy monitor")
	// Now create new monitor policy
	data = fmt.Sprintf(monitorPolicyData, c.tenantId, resp.ID, resp.ID)
	_ = doReq("POST", policyUrl, data, "creating monitor policy", c.adminToken)

}

func checkPresenceMonitor(c config) {
	url := c.adminApiUrl + "api/presence-monitor/partitions/"
	_ = doReq("GET", url, "", "getting presence monitor partitions", c.adminToken)
	log.Println("got presence monitor partitions")
}
