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
	"fmt"
	"github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"io/ioutil"
	"log"
	"net/http"
)

const (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	id := uuid.NewV1()
	privateZoneId := "privateZone" + id.String()
	tenantId := "aaaaaa"

	message := `{"name": "` + privateZoneId + `"}`


	req, err := http.NewRequest("POST", "http://localhost:8080/v1.0/tenant/" + tenantId + "/zones", bytes.NewBuffer([]byte(message)))
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
    Brokers:   []string{"localhost:9092"},
    Topic:     "telemetry.resources.json",
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
