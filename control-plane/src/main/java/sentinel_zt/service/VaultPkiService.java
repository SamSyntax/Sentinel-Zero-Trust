package sentinel_zt.service;

import java.util.Map;

import org.slf4j.MDC;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.stereotype.Service;
import org.springframework.vault.core.VaultTemplate;
import org.springframework.vault.support.VaultResponse;

import lombok.extern.slf4j.Slf4j;
import sentinel_zt.dto.IdentityResponse;

@Service
@ConditionalOnProperty(name = "spring.cloud.vault.enabled", havingValue = "true", matchIfMissing = true)
@Slf4j
public class VaultPkiService {

  private final VaultTemplate vaultTemplate;
  private final AuditService auditService;

  public VaultPkiService(VaultTemplate vaultTemplate, AuditService auditService) {
    this.vaultTemplate = vaultTemplate;
    this.auditService = auditService;
  }

  public IdentityResponse issueCertificate(String serviceName) {
    String traceId = MDC.get("trace_id");
    String requestId = MDC.get("request_id");
    String serviceIdentity = MDC.get("serviceIdentity");
    
    log.info("Certificate issuance requested - serviceName={}, requestedBy={}, traceId={}, requestId={}",
            serviceName, serviceIdentity, traceId, requestId);
    
    long startTime = System.currentTimeMillis();
    
    Map<String,Object> request = Map.of(
        "common_name",  getServiceAccountName(serviceName),
        "alt_names", serviceIdentity,
        "ttl", "6m"
        );
    
    try {
      VaultResponse response = vaultTemplate.write("pki/issue/sentinel-service", request);
      Map<String, Object> data = response.getData();
      
      String serialNumber = (String) data.get("serial_number");
      
      IdentityResponse identityResponse = IdentityResponse.builder()
        .certificate((String) data.get("certificate"))
        .privateKey((String) data.get("private_key"))
        .issuingCa((String) data.get("issuing_ca"))
        .serialNumber(serialNumber)
        .build();

      long duration = System.currentTimeMillis() - startTime;
      
      log.info("Certificate issued successfully - serviceName={}, serialNumber={}, durationMs={}, traceId={}, requestId={}",
              serviceName, serialNumber, duration, traceId, requestId);
      
      auditService.logCertificateIssued(serviceName, serialNumber, serviceIdentity);
      
      return identityResponse;

    } catch (Exception e) {
      long duration = System.currentTimeMillis() - startTime;
      
      log.error("Certificate issuance failed - serviceName={}, error={}, durationMs={}, traceId={}, requestId={}, stackTrace={}",
              serviceName, e.getMessage(), duration, traceId, requestId, e.getStackTrace());
      
      auditService.logSecurityEvent("CERTIFICATE_ISSUANCE_FAILED", Map.of(
          "serviceName", serviceName,
          "error", e.getMessage(),
          "timestamp", java.time.Instant.now().toString()
      ));
      
      return null;
    }
  }

  public VaultTemplate getVaultTemplate() {
    return vaultTemplate;
  }

  public String getServiceAccountName(String identity) {
    var parts = identity.split("/");
    if (parts.length == 0) {
      return null;
    }
    var serviceAccount = parts[parts.length - 1];
    return serviceAccount;
  }
}
