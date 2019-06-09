import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.github.jknack.handlebars.Context;

import com.github.jknack.handlebars.Handlebars;
import com.github.jknack.handlebars.Helper;
import com.github.jknack.handlebars.Jackson2Helper;
import com.github.jknack.handlebars.Options;
import com.github.jknack.handlebars.Template;
import com.github.jknack.handlebars.context.FieldValueResolver;
import com.github.jknack.handlebars.context.JavaBeanValueResolver;
import com.github.jknack.handlebars.context.MapValueResolver;
import com.github.jknack.handlebars.context.MethodValueResolver;
import com.github.jknack.handlebars.JsonNodeValueResolver;

import com.github.jknack.handlebars.helper.StringHelpers;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Paths;

public class InvokeHandlebars {

  public static void main(String[] args) throws IOException {
    String json = new String ( Files.readAllBytes( Paths.get("/Users/geor7956/incoming/s4/salus-telemetry-bundle/apps/monitor-management/target/generated/swagger/swagger.json") ) );
    //String json = "{\"swagger\": \"world\"}";

    JsonNode jsonNode = new ObjectMapper().readValue(json, JsonNode.class);
    Handlebars handlebars = new Handlebars();
    handlebars.registerHelper("json", Jackson2Helper.INSTANCE);
  //  handlebars.registerHelper("join", StringHelpers.join );

    handlebars.registerHelper("ifeq", new Helper<String>() {
      @Override
      public CharSequence apply(String value, Options options) throws IOException {
        if (value == null || options.param(0) == null) {
          return options.inverse();
        }
        if (value.equals(options.param(0))) {
          return options.fn();
        }
        return options.inverse();
      }
    });

    handlebars.registerHelper("basename", new Helper<String>() {
      @Override
      public CharSequence apply(String value, Options options) throws IOException {
        if (value == null) {
          return null;
        }
        int lastSlash = value.lastIndexOf("/");
        if (lastSlash == -1) {
          return value;
        } else {
          return value.substring(lastSlash + 1);
        }
      }
    });

    handlebars.registerHelper(StringHelpers.join.name(), StringHelpers.join);
    handlebars.registerHelper(StringHelpers.lower.name(), StringHelpers.lower);



    Context context = Context
        .newBuilder(jsonNode)
        .resolver(JsonNodeValueResolver.INSTANCE,
            JavaBeanValueResolver.INSTANCE,
            FieldValueResolver.INSTANCE,
            MapValueResolver.INSTANCE,
            MethodValueResolver.INSTANCE
        )
        .build();
//    Template template = handlebars.compileInline("Hello1 {{swagger}}!");
    String tString = new String(Files.readAllBytes(Paths.get("/Users/geor7956/incoming/s4/salus-telemetry-bundle/apps/monitor-management/templates/strapdown.html.hbs")));
      Template template = handlebars.compileInline(tString);
      System.out.println(template.apply(context));



  }

}
