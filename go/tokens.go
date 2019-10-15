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
	"fmt"
	"encoding/json"
)
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
