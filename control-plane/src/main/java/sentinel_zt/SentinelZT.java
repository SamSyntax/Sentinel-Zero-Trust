package sentinel_zt;

import lombok.extern.slf4j.Slf4j;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.SpringApplication;

@Slf4j
@SpringBootApplication(excludeName = {
    "org.springframework.boot.autoconfigure.security.oauth2.client.servlet.OAuth2ClientAutoConfiguration"
})
public class SentinelZT {
  public static void main(String[] args) {
    try {
      SpringApplication.run(SentinelZT.class, args);
    } catch (Exception e) {
      if (e.getClass().getName().contains("SilentExitException")) {
        throw e;
      }
      log.error("Failed to start SentinelZT application: {}", e.getMessage(), e);
      System.exit(1);
    }
  }
}
