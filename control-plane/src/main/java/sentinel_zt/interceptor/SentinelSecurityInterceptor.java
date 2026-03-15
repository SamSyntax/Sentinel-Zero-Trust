package sentinel_zt.interceptor;

import java.util.UUID;

import org.slf4j.MDC;
import org.springframework.stereotype.Component;
import org.springframework.web.servlet.HandlerInterceptor;

import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import lombok.extern.slf4j.Slf4j;
import sentinel_zt.service.AuditService;
import sentinel_zt.service.K8sTokenReviewService;

@Slf4j
@Component
public class SentinelSecurityInterceptor implements HandlerInterceptor {
  private final K8sTokenReviewService k8sService;
  private final AuditService auditService;
  
  public SentinelSecurityInterceptor(K8sTokenReviewService k8sService, AuditService auditService) {
    this.k8sService = k8sService;
    this.auditService = auditService;
  }
  
  @Override
  public boolean preHandle(HttpServletRequest request, HttpServletResponse response, Object handler) {
    String traceId = MDC.get("trace_id");
    String requestId = MDC.get("request_id");
    
    String tokenHeader = request.getHeader("X-Sentinel-Token");
    if(tokenHeader == null || !tokenHeader.startsWith("Bearer ")) {
      log.warn("Missing or invalid token header - uri={}, method={}, traceId={}, requestId={}, clientIp={}",
              request.getRequestURI(), request.getMethod(), traceId, requestId, getClientIp(request));
      auditService.logUnauthorizedAccessAttempt(
              request.getRequestURI(),
              getClientIp(request),
              request.getHeader("User-Agent"));
      response.setStatus(HttpServletResponse.SC_UNAUTHORIZED);
      return false;
    }
    String token = tokenHeader.substring(7);

    long startTime = System.currentTimeMillis();
    try {
      if(k8sService.validateToken(token)) {
        String serviceName = k8sService.getServiceAccountName(token);
        request.setAttribute("verifiedService", serviceName);
        MDC.put("serviceIdentity", serviceName);
        MDC.put("traceId", UUID.randomUUID().toString());
        
        log.info("Zero-trust request authorized - serviceName={}, uri={}, method={}, traceId={}, requestId={}",
                serviceName, request.getRequestURI(), request.getMethod(), traceId, requestId);
        
        auditService.logAuthenticationAttempt(serviceName, true, null);
        return true;
      } 
      
      log.error("Token validation failed - uri={}, method={}, traceId={}, requestId={}, reason=invalid_token",
              request.getRequestURI(), request.getMethod(), traceId, requestId);
      
      auditService.logAuthenticationAttempt("unknown", false, "invalid_token");
      response.setStatus(HttpServletResponse.SC_UNAUTHORIZED);
      return false;
      
    } catch (Exception e) {
      log.error("Token validation error - uri={}, method={}, traceId={}, requestId={}, error={}, stackTrace={}",
              request.getRequestURI(), request.getMethod(), traceId, requestId, e.getMessage(), e.getStackTrace());
      
      auditService.logAuthenticationAttempt("unknown", false, "validation_error: " + e.getMessage());
      response.setStatus(HttpServletResponse.SC_UNAUTHORIZED);
      return false;
    } finally {
      long duration = System.currentTimeMillis() - startTime;
      log.debug("Token validation completed in {}ms - traceId={}, requestId={}", duration, traceId, requestId);
    }
  }

  @Override
  public void afterCompletion(HttpServletRequest request, HttpServletResponse response, Object handler, Exception ex) {
    MDC.remove("serviceIdentity");
    MDC.remove("traceId");
  }
  
  private String getClientIp(HttpServletRequest request) {
    String xForwardedFor = request.getHeader("X-Forwarded-For");
    if (xForwardedFor != null && !xForwardedFor.isEmpty()) {
      return xForwardedFor.split(",")[0].trim();
    }
    return request.getRemoteAddr();
  }
}
