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
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)
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
	defer closeResp(resp)
	body, err := ioutil.ReadAll(resp.Body)
	checkErr(err, "unable read response body: "+errMessage)
	if resp.StatusCode/100 != 2 {
		log.Println(errMessage + ": " + string(body))
		log.Fatal("status code: " + resp.Status)
	}

	return body
}
