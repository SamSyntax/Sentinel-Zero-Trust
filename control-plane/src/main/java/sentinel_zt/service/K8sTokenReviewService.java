package sentinel_zt.service;

import org.slf4j.MDC;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

import com.fasterxml.jackson.databind.ObjectMapper;

import io.fabric8.kubernetes.api.model.authentication.TokenReview;
import io.fabric8.kubernetes.api.model.authentication.TokenReviewBuilder;
import io.fabric8.kubernetes.client.KubernetesClient;
import lombok.extern.slf4j.Slf4j;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonMappingException;
import com.fasterxml.jackson.databind.JsonNode;
@Service
@Slf4j
public class K8sTokenReviewService {
  private final KubernetesClient client;
  private final AuditService auditService;
  @Value("${SPIFFE_TRUST_DOMAIN:cluster.local}")
  private String trustDomain;
  
  public K8sTokenReviewService(KubernetesClient client, AuditService  auditService) {
    this.client = client;
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

  public SpiffeIdentity getSpiffeIdentity(String token) throws JsonMappingException, JsonProcessingException {
    log.debug(">>>> StringToken={}", token);
  java.util.Base64.Decoder decoder = java.util.Base64.getDecoder();
  String[] parts = token.split("\\.");
  if (parts.length != 3) {
    return null;
  }
  String  payload = new String(decoder.decode(parts[1]));
  ObjectMapper mapper = new ObjectMapper();
  JsonNode root = mapper.readTree(payload);
  JsonNode kubernetes = root.get("kubernetes.io");
  String namespace = kubernetes.get("namespace").asText();
  String serviceAccount = kubernetes.get("serviceaccount").get("name").asText();
  log.debug(">>>> Namespace={}, ServiceAccount={}", namespace, serviceAccount);

  String spiffeId = String.format("spiffe://%s/ns/%s/sa/%s", trustDomain, namespace, serviceAccount);

  return new SpiffeIdentity(namespace, serviceAccount, spiffeId);

  }
  public record SpiffeIdentity(String namespace, String serviceAccount, String spiffeId) {}
}
