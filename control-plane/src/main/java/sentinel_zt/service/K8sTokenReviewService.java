package sentinel_zt.service;

import org.slf4j.MDC;
import org.springframework.stereotype.Service;

import io.fabric8.kubernetes.api.model.authentication.TokenReview;
import io.fabric8.kubernetes.api.model.authentication.TokenReviewBuilder;
import io.fabric8.kubernetes.client.KubernetesClient;
import io.fabric8.kubernetes.client.KubernetesClientBuilder;
import lombok.extern.slf4j.Slf4j;

@Service
@Slf4j
public class K8sTokenReviewService {
  private final KubernetesClient client;
  private final AuditService auditService;
  
  public K8sTokenReviewService(AuditService auditService) {
    this.client = new KubernetesClientBuilder().build();
    this.auditService = auditService;
  }

  public boolean validateToken(String token) {
    String traceId = MDC.get("trace_id");
    String requestId = MDC.get("request_id");
    
    log.debug("Validating Kubernetes token - traceId={}, requestId={}", traceId, requestId);
    
    long startTime = System.currentTimeMillis();
    
    try {
      TokenReview tr = new TokenReviewBuilder()
        .withNewSpec()
        .withToken(token)
        .endSpec()
        .build();

      TokenReview result = client.authentication().v1().tokenReviews().create(tr);

      boolean authenticated = result.getStatus() != null && result.getStatus().getAuthenticated();
      
      long duration = System.currentTimeMillis() - startTime;
      
      if (authenticated) {
        log.debug("Token validation successful - durationMs={}, traceId={}, requestId={}",
                duration, traceId, requestId);
      } else {
        log.warn("Token validation failed - durationMs={}, traceId={}, requestId={}",
                duration, traceId, requestId);
      }
      
      auditService.logTokenValidation(
              result.getStatus() != null ? result.getStatus().getUser().getUsername() : "unknown",
              authenticated,
              authenticated ? "success" : "not_authenticated");
      
      return authenticated;

    } catch (Exception e) {
      long duration = System.currentTimeMillis() - startTime;
      
      log.error("Token validation error - error={}, durationMs={}, traceId={}, requestId={}, stackTrace={}",
              e.getMessage(), duration, traceId, requestId, e.getStackTrace());
      
      auditService.logTokenValidation("unknown", false, "error: " + e.getMessage());
      
      return false;
    }
  }

  public String getServiceAccountName(String token) {
    String traceId = MDC.get("trace_id");
    String requestId = MDC.get("request_id");
    
    log.debug("Extracting service account name from token - traceId={}, requestId={}", traceId, requestId);
    
    try {
      TokenReview result = client.authentication().v1().tokenReviews().create(
          new TokenReviewBuilder().withNewSpec().withToken(token).endSpec().build()
          );
      
      String username = result.getStatus().getUser().getUsername();
      
      log.debug("Service account name extracted - serviceAccount={}, traceId={}, requestId={}",
              username, traceId, requestId);
      
      return username;
      
    } catch (Exception e) {
      log.error("Failed to extract service account name - error={}, traceId={}, requestId={}, stackTrace={}",
              e.getMessage(), traceId, requestId, e.getStackTrace());
      
      return "unknown";
    }
  }
}
