package sentinel_zt.service;

import lombok.extern.slf4j.Slf4j;
import org.slf4j.MDC;
import org.springframework.stereotype.Service;

import java.time.Instant;
import java.util.Map;

@Service
@Slf4j
public class AuditService {

    public void logCertificateIssued(String serviceName, String serialNumber, String requestedBy) {
        String traceId = MDC.get("trace_id");
        String requestId = MDC.get("request_id");
        
        log.info("AUDIT: Certificate issued - serviceName={}, serialNumber={}, requestedBy={}, traceId={}, requestId={}",
                serviceName,
                serialNumber,
                requestedBy,
                traceId,
                requestId);
        
        logAuditEvent("CERTIFICATE_ISSUED", Map.of(
            "serviceName", serviceName != null ? serviceName : "unknown",
            "serialNumber", serialNumber != null ? serialNumber : "unknown",
            "requestedBy", requestedBy != null ? requestedBy : "unknown",
            "timestamp", Instant.now().toString()
        ));
    }

    public void logTokenValidation(String tokenSubject, boolean success, String reason) {
        String traceId = MDC.get("trace_id");
        String requestId = MDC.get("request_id");
        
        if (success) {
            log.info("AUDIT: Token validated successfully - subject={}, traceId={}, requestId={}",
                    tokenSubject, traceId, requestId);
        } else {
            log.warn("AUDIT: Token validation failed - subject={}, reason={}, traceId={}, requestId={}",
                    tokenSubject, reason, traceId, requestId);
        }
        
        logAuditEvent("TOKEN_VALIDATION", Map.of(
            "tokenSubject", tokenSubject != null ? tokenSubject : "unknown",
            "success", success,
            "reason", reason != null ? reason : "none",
            "timestamp", Instant.now().toString()
        ));
    }

    public void logAuthenticationAttempt(String serviceAccount, boolean success, String failureReason) {
        String traceId = MDC.get("trace_id");
        String requestId = MDC.get("request_id");
        
        if (success) {
            log.info("AUDIT: Authentication successful - serviceAccount={}, traceId={}, requestId={}",
                    serviceAccount, traceId, requestId);
        } else {
            log.warn("AUDIT: Authentication failed - serviceAccount={}, reason={}, traceId={}, requestId={}",
                    serviceAccount, failureReason, traceId, requestId);
        }
        
        logAuditEvent("AUTHENTICATION_ATTEMPT", Map.of(
            "serviceAccount", serviceAccount != null ? serviceAccount : "unknown",
            "success", success,
            "failureReason", failureReason != null ? failureReason : "none",
            "timestamp", Instant.now().toString()
        ));
    }

    public void logUnauthorizedAccessAttempt(String endpoint, String clientIp, String userAgent) {
        String traceId = MDC.get("trace_id");
        String requestId = MDC.get("request_id");
        
        log.warn("AUDIT: Unauthorized access attempt - endpoint={}, clientIp={}, userAgent={}, traceId={}, requestId={}",
                endpoint, clientIp, userAgent, traceId, requestId);
        
        logAuditEvent("UNAUTHORIZED_ACCESS", Map.of(
            "endpoint", endpoint != null ? endpoint : "unknown",
            "clientIp", clientIp != null ? clientIp : "unknown",
            "userAgent", userAgent != null ? userAgent : "unknown",
            "timestamp", Instant.now().toString()
        ));
    }

    public void logSecurityEvent(String eventType, Map<String, Object> details) {
        String traceId = MDC.get("trace_id");
        String requestId = MDC.get("request_id");
        
        log.warn("AUDIT: Security event - eventType={}, details={}, traceId={}, requestId={}",
                eventType, details, traceId, requestId);
        
        logAuditEvent(eventType, details);
    }

    private void logAuditEvent(String eventType, Map<String, Object> details) {
        log.debug("Audit event details: eventType={}, details={}", eventType, details);
    }
}
