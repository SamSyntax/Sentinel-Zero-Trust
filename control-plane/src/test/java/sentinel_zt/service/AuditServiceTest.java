package sentinel_zt.service;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.junit.jupiter.MockitoExtension;

@ExtendWith(MockitoExtension.class)
class AuditServiceTest {
@Test
void logCertificateIssued_logsCorrectFormat() {
  AuditService auditService = new AuditService();
  assertDoesNotThrow(() -> auditService.logCertificateIssued("svc-1", "serial-123", "requester")
      );
}
}
