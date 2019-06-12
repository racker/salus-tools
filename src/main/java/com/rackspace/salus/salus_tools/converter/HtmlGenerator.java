package com.rackspace.salus.salus_tools.converter;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.github.kongchen.swagger.docgen.AbstractDocumentSource;
import com.github.kongchen.swagger.docgen.mavenplugin.ApiSource;
import com.github.kongchen.swagger.docgen.reader.ClassSwaggerReader;
import io.swagger.models.Swagger;
import io.swagger.util.DeserializationModule;
import org.apache.maven.plugin.logging.Log;


import java.util.Set;
// Do just enough to allow creation of concrete child class
public class HtmlGenerator extends AbstractDocumentSource {

  HtmlGenerator(ApiSource apiSource, Log log, String swagger) throws Exception {

    super(log, apiSource);
    ObjectMapper objectMapper = new ObjectMapper();
    DeserializationModule dm = new DeserializationModule();
    objectMapper.registerModule(dm);
    this.swagger = objectMapper.readValue(swagger, Swagger.class);
  }

  @Override
  protected Set<Class<?>> getValidClasses() {
    return null;
  }


  @Override
  protected ClassSwaggerReader resolveApiReader() {
   return null;
  }

}
