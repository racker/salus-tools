package com.rackspace.salus.salus_tools.converter;



import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;

import java.io.File;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.HashMap;
import java.util.Iterator;
import java.util.Map;
import java.util.Scanner;

public class SwaggerJsonConverter {
    static final String argDelimiter="=";

    /**
     * This function
     * @param args
     * 0. path to the location that the swagger.json is and to where we will output the resultant json from this
     * 1. should be "tenant/{tenantId}"= or {tenandId}= depending on whether the API has /tenant in the path
     * 2. Each argument should be in the format "StringToBeReplaced"="replacementText".
     *    If you want to remove a string then it should be "StringToBeReplaced"=    with no replacmentText
     * @throws Exception
     */
    public static void main(String[] args) throws Exception {
        ObjectMapper mapper = new ObjectMapper();
        String content = new Scanner(new File(args[0]+"/swagger.json")).useDelimiter("\\Z").next();
        ObjectNode root = (ObjectNode)mapper.readTree(content);
        Map<String, JsonNode> temp = new HashMap();

        String newKey = null;
        boolean containsTenant;
        for (Iterator<Map.Entry<String, JsonNode>> it = root.get("paths").fields(); it.hasNext(); ) {
            Map.Entry<String, JsonNode> elt = it.next();
            newKey = elt.getKey();
            containsTenant = newKey.contains("tenant");
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
                temp.put(newKey, elt.getValue());
            }

            it.remove();
        }
        ObjectNode pathNode = mapper.getNodeFactory().objectNode();

        temp.forEach((key, node)->{
            pathNode.set(key, node);
        });
        root.set("paths", pathNode);
        // if this fails to create the directory then the public documentation wont get generated
        Path parent= Files.createDirectory(Paths.get(args[0] +"/public/"));
        mapper.writeValue(parent.resolve("swagger.json").toFile(), (JsonNode) root);

    }
}
