/*
 * Copyright 2020 Rackspace US, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package com.rackspace.salus.salus_tools.converter;



import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.io.File;
import java.util.HashMap;
import java.util.Iterator;
import java.util.Map;
import java.util.Scanner;

public class SwaggerJsonConverter {
    private static final String argDelimiter="=";

    /**
     * This function will generate both public and admin swagger json splitting it where the public docs have
     * /tenant/{tenantId} or {tenantId} in the path and assume that anything else is an admin api
     * @param args
     * 0. path to the location that the swagger.json is and to where we will output the resultant json
     * 1. should be "tenant/{tenantId}"= or {tenandId}= depending on whether the API has /tenant in the path
     * 2. Each succeeding argument should be in the format "StringToBeReplaced"="replacementText".
     *    If you want to remove a string then it should be "StringToBeReplaced"=    with no replacmentText
     * @throws Exception
     */
    public static void main(String[] args) throws Exception {
        ObjectMapper mapper = new ObjectMapper();
        String content = new Scanner(new File(args[0]+"/swagger.json")).useDelimiter("\\Z").next();
        ObjectNode publicRoot = (ObjectNode)mapper.readTree(content);
        ObjectNode adminRoot = (ObjectNode)mapper.readTree(content);
        Map<String, JsonNode> publicTemp = new HashMap<>();
        Map<String, JsonNode> adminTemp = new HashMap<>();

        for (Iterator<Map.Entry<String, JsonNode>> it = publicRoot.get("paths").fields(); it.hasNext(); ) {
            Map.Entry<String, JsonNode> elt = it.next();
            String newKey = elt.getKey();
            boolean containsTenant = newKey.contains("tenant");
            for(int i = 1; i < args.length; i++) {
                String[] splitValues = args[i].split(argDelimiter);
                newKey = newKey.replace(splitValues[0], splitValues.length == 1? "" : splitValues[1]);
            }
            /*
            attempting to remove the parameters but is failing on array index out of bounds exception

            elt.getValue().fields().forEachRemaining((webVerbs)->{
                int i = 0;
                for (Iterator<Map.Entry<String, JsonNode>> parameters = elt.getValue().get("parameters").fields(); parameters.hasNext(); ) {
                    Map.Entry<String, JsonNode> parameter = parameters.next();
                    if (parameter.getValue().get("name").asText().compareTo("tenantId") == 0) {
                        //parameters is an array
                        ((ArrayNode)temp.get(webVerbs.getKey()).get("parameters")).remove(i);
                        break;
                    }
                    i++;
                }
            });*/
            if(containsTenant) {
              publicTemp.put(newKey, elt.getValue());
            }else {
              adminTemp.put(newKey, elt.getValue());
            }

            it.remove();
        }
        ObjectNode publicPathNode = mapper.getNodeFactory().objectNode();
        ObjectNode adminPathNode = mapper.getNodeFactory().objectNode();

        publicTemp.forEach((key, node)->{
            publicPathNode.set(key, node);
        });

        adminTemp.forEach((key, node)-> {
          adminPathNode.set(key, node);
        });

        // generate the admin swagger json
        File adminDir = new File(args[0],"admin");
        adminDir.mkdirs();
        adminRoot.set("paths", adminPathNode);
        mapper.writeValue(new java.io.File(adminDir,"swagger.json"), (JsonNode)adminRoot);

        // generate the public swagger json
        File publicDir = new File(args[0],"public");
        publicDir.mkdirs();
        publicRoot.set("paths", publicPathNode);
        mapper.writeValue(new java.io.File(publicDir, "swagger.json"), (JsonNode)publicRoot);
    }
}
