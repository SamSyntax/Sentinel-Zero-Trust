package sentinel_zt.config;

import org.springframework.context.annotation.Configuration;
import org.springframework.web.servlet.config.annotation.InterceptorRegistry;
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import sentinel_zt.interceptor.SentinelSecurityInterceptor;

@Configuration
public class WebConfig implements WebMvcConfigurer {
  private final SentinelSecurityInterceptor sentinelInterceptor;
  private final RequestLoggingInterceptor requestLoggingInterceptor;
  private static final Logger log = LoggerFactory.getLogger(WebConfig.class);

  public WebConfig(SentinelSecurityInterceptor sentinelInterceptor, 
                   RequestLoggingInterceptor requestLoggingInterceptor) {
    this.sentinelInterceptor = sentinelInterceptor;
    this.requestLoggingInterceptor = requestLoggingInterceptor;
  }

  @Override
  public void addInterceptors(InterceptorRegistry registry) {
    registry.addInterceptor(requestLoggingInterceptor)
            .addPathPatterns("/api/**")
            .excludePathPatterns("/actuator/**", "/health", "/ready");
    
    registry.addInterceptor(sentinelInterceptor)
            .addPathPatterns("/api/v1/identity/**");
    
    log.info("Registered interceptors: RequestLoggingInterceptor, SentinelSecurityInterceptor");
  }
}
