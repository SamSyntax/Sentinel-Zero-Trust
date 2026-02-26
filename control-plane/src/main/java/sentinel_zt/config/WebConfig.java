package sentinel_zt.config;

import org.springframework.context.annotation.Configuration;
import org.springframework.web.servlet.config.annotation.InterceptorRegistry;
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer;

import lombok.extern.slf4j.Slf4j;
import sentinel_zt.interceptor.SentinelSecurityInterceptor;

@Slf4j
@Configuration
public class WebConfig implements WebMvcConfigurer {
  private final SentinelSecurityInterceptor sentinelInterceptor;
  private final RequestLoggingInterceptor requestLoggingInterceptor;

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
