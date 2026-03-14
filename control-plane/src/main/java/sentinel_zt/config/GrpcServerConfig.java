package sentinel_zt.config;


import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.context.annotation.Configuration;

/**
 * gRPC Server Configuration.
 * 
 * The gRPC server lifecycle is managed automatically by Spring gRPC's
 * autoconfiguration. Services annotated with @GrpcService (like IdentityService)
 * are auto-discovered and registered. Server port/address are configured via
 * application.yml under spring.grpc.server.*.
 *
 * This config class is retained for any future custom gRPC server tuning
 * (e.g., interceptors, custom server builders).
 */
@Configuration
public class GrpcServerConfig {
  private static final Logger log = LoggerFactory.getLogger(GrpcServerConfig.class);

  public GrpcServerConfig() {
    log.info("gRPC server configuration loaded — server lifecycle managed by Spring gRPC autoconfiguration");
  }
}
