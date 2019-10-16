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
	"path"
)

func initConfig() config {
	replacer := strings.NewReplacer(".", "_", "-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetEnvPrefix("E2ET")
	viper.AutomaticEnv() // read in environment variables that match

	cfgFile := flag.String("config", "config.yml", "config file")
	flag.Parse()
	viper.SetConfigFile(*cfgFile)
	err := viper.ReadInConfig()
	checkErr(err, "Config file not found "+*cfgFile)
	log.Println("loaded: " + *cfgFile)
	var c config
	c.mode = viper.GetString("mode")
	c.currentUUID = uuid.NewV4()
	c.id = strings.Replace(c.currentUUID.String(), "-", "", -1)
	c.privateZoneId = "privateZone_" + c.id
	c.resourceId = "resourceId_" + c.id
	c.tenantId = viper.GetString("tenantId")
	c.publicApiUrl = viper.GetString("publicApiUrl")
	c.adminApiUrl = viper.GetString("adminApiUrl")
	c.agentReleaseUrl = c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/agent-releases"
	certDir := viper.GetString("certDir")

	if !strings.HasPrefix(certDir, "/") {
		wd, err := os.Getwd()
		checkErr(err, "getting working dir")
		certDir = path.Join(wd, certDir)
	}
	c.certDir = certDir
	c.regularId = viper.GetString("regularId")
	c.adminId = viper.GetString("adminId")
	dir, err := ioutil.TempDir("", "e2et")
	checkErr(err, "error creating temp dir")
	c.dir = dir
	c.kafkaBrokers = viper.GetStringSlice("kafkaBrokers")
	c.eventTopic = viper.GetString("eventTopic")
	c.identityUrl = viper.GetString("identityUrl")
	c.authUrl = viper.GetString("authUrl")
	c.ambassadorAddress = viper.GetString("ambassadorAddress")
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
	deletePrivateZones(c)
	createPrivateZone(c)

	cmd := initEnvoy(c, releaseId)
	defer killCmd(cmd)
	deleteTasks(c)
	createTasks(c)
	eventFound := make(chan bool, 1)
	go checkForEvents(c, eventFound)
	createMonitors(c)
	deletePolicyMonitors(c)
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

func killCmd(cmd *exec.Cmd) {
	err := cmd.Process.Kill()
	checkErr(err, "killing envoy")
}

func initEnvoy(c config, releaseId string) (cmd *exec.Cmd) {
	log.Println("starting envoy")
	configFileName := c.dir + "/config.yml"
	f, err := os.Create(configFileName)
	checkErr(err, "create envoy config file: "+configFileName)
	var configTemplate string
	if c.mode == "local" {
		configTemplate = localConfigTemplate
	} else {
		configTemplate = remoteConfigTemplate
	}
	tmpl, err := template.New("t1").Parse(configTemplate)
	checkErr(err, "parsing envoy template")

	err = tmpl.Execute(f, TemplateFields{c.resourceId, c.privateZoneId,
		c.certDir, c.regularApiKey, c.regularId, c.authUrl, c.ambassadorAddress})
	checkErr(err, "creating envoy template")
	err = f.Close()
	checkErr(err, "closing envoy config file: " + configFileName)
	cmd = exec.Command(os.Getenv("GOPATH")+"/bin/telemetry-envoy", "run", "--config="+configFileName)
	cmd.Dir = c.dir
	cmd.Stdout, err = os.Create(c.dir + "/envoyStdout.log")
	checkErr(err, "redirecting stdout")
	cmd.Stderr, err = os.Create(c.dir + "/envoyStderr.log")
	checkErr(err, "redirecting stderr")
	err = cmd.Start()
	checkErr(err, "starting envoy")

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
	_, err = os.Stat(c.dir + "/data-telemetry-envoy")
	checkErr(err, "envoy failed")
	log.Println("envoy started")
	return cmd
}

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

func closeResp(resp *http.Response) {
	err := resp.Body.Close()
	checkErr(err, "closing resp body")
}

func deleteAgentInstalls(c config) {
	log.Println("deleting AgentInstalls")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/agent-installs/"
	
	for page := 0; ;page += 1 {
		pageStr := fmt.Sprintf("?page=%d", page)
		installBody := doReq("GET", url + pageStr,
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
	for page := 0; ;page += 1 {
		pageStr := fmt.Sprintf("?page=%d", page)
		body := doReq("GET", url + pageStr,
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
	for page := 0; ;page += 1 {
		pageStr := fmt.Sprintf("?page=%d", page)
		body := doReq("GET", url + pageStr,
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

func deletePrivateZones(c config) {
	log.Println("deleting private zones")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/zones/"
	for page := 0; ;page += 1 {
		pageStr := fmt.Sprintf("?page=%d", page)
		body := doReq("GET", url + pageStr,
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
}

func createPrivateZone(c config) {
	log.Println("creating private zone")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/zones/"
	message := `{"name": "` + c.privateZoneId + `"}`
	log.Println("creating zone: %s %s", url, message)
	_ = doReq("POST", url, message, "creating private zone", c.regularToken)
}

func deleteTasks(c config) {
	log.Println("deleting Tasks")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/event-tasks/"
	for page := 0; ;page += 1 {
		pageStr := fmt.Sprintf("?page=%d", page)
		body := doReq("GET", url + pageStr,
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
}

func createTasks(c config) {
	log.Println("create Tasks")
	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/event-tasks/"
	data := fmt.Sprintf(taskData, "net_response_task_"+c.id, "net_response", runtime.GOOS)
	_ = doReq("POST", url, data, "creating net task", c.regularToken)
	data = fmt.Sprintf(taskData, "http_response_task_"+c.id, "http_response", runtime.GOOS)
	_ = doReq("POST", url, data, "creating http task", c.regularToken)
}

func checkForEvents(c config, eventFound chan bool) {
	var r *kafka.Reader
	finishedMap := make(map[string]bool)
	finishedMap["net"] = false
	if c.mode == "local" {
		r = kafka.NewReader(kafka.ReaderConfig{
			Brokers:  c.kafkaBrokers,
			Topic:    c.eventTopic,
			MinBytes: 1,
			MaxBytes: 10e6, // 10MB
		})

	} else {
		finishedMap["http"] = false
		cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
		checkErr(err, "loading client cert")
		caCert, err := ioutil.ReadFile(c.caFile)
		checkErr(err, "loading ca cert")
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
			Topic:    c.eventTopic,
			MinBytes: 1,
			MaxBytes: 10e6, // 10MB
			Dialer:   dialer,
		})
	}
	log.Println("waiting for events")
	for {
		m, err := r.ReadMessage(context.Background())
		checkErr(err, "reading kafka")
		log.Printf("message at topic/partition/offset %v/%v/%v: %s = %s\n",
			m.Topic, m.Partition, m.Offset, string(m.Key), string(m.Value))
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
				eventFound<-true
			}
		}
	}

}

func createMonitors(c config) {
	log.Println("creating Monitors")

	url := c.publicApiUrl + "v1.0/tenant/" + c.tenantId + "/monitors/"
	data := fmt.Sprintf(netMonitorData, runtime.GOOS, c.privateZoneId, c.port)
	_ = doReq("POST", url, data, "creating net monitor", c.regularToken)
	log.Println("monitors created")

}

func deletePolicyMonitors(c config) {
	// NOTE: Figure out why this doesn't work with private pollers
	if c.mode == "local" {
		return
	}
	log.Println("deleting policy monitors")
	policyUrl := c.adminApiUrl + "api/policies/monitors/"
	monitorUrl := c.adminApiUrl + "api/policy-monitors/"

	for page := 0; ;page += 1 {
		pageStr := fmt.Sprintf("?page=%d", page)
		body := doReq("GET", policyUrl + pageStr,
			"", "getting all policy monitors", c.adminToken)
		var resp GetPoliciesResp
		err := json.Unmarshal(body, &resp)
		checkErr(err, "unable to parse get policy monitor response")
		for _, i := range resp.Content {
			// Only delete policies for this tenant
			if i.Subscope != c.tenantId {
				continue
			}
		
			// delete each policy
			_ = doReq("DELETE", policyUrl+i.ID, "", "deleting policy "+i.ID, c.adminToken)
			// delete the corresponding monitor
			_ = doReq("DELETE", monitorUrl+i.MonitorID, "", "deleting policy monitor "+i.MonitorID, c.adminToken)
		}
		if resp.Last {
			break
		}
	}
}

func createPolicyMonitor(c config) {
	// NOTE: Figure out why this doesn't work with private pollers
	if c.mode == "local" {
		return
	}
	log.Println("creating policy monitors")
	policyUrl := c.adminApiUrl + "api/policies/monitors/"
	monitorUrl := c.adminApiUrl + "api/policy-monitors/"

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
