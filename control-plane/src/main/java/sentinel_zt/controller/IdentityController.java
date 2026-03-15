package sentinel_zt.controller;

import sentinel_zt.dto.IdentityResponse;
import sentinel_zt.dto.IdentityRequest;
import sentinel_zt.service.VaultPkiService;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.validation.Valid;
import lombok.extern.slf4j.Slf4j;

import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@Slf4j
@RestController
@ConditionalOnProperty(name = "spring.cloud.vault.enabled", havingValue = "true", matchIfMissing = true)
@RequestMapping("/api/v1/identity")
public class IdentityController {
  private final VaultPkiService pkiService;

  public IdentityController(VaultPkiService pkiService) {
    this.pkiService = pkiService;
  }

  @PostMapping("/issue")
  public ResponseEntity<IdentityResponse> issueIdentity(@Valid @RequestBody IdentityRequest request, @RequestHeader(value = "X-Sentinel-Token", required = true) String authHeader, HttpServletRequest httpRequest) {
    String callerIdentity = (String) httpRequest.getAttribute("verifiedService");
    String requestedService = request.getServiceName();
    if (callerIdentity == null || !callerIdentity.equals(requestedService)) {
      log.warn("Identity spoofing attempt - caller={}, requested={}", callerIdentity, requestedService);
      return ResponseEntity.status(HttpStatus.FORBIDDEN).build();
    }
    IdentityResponse response = pkiService.issueCertificate(requestedService);
    if (response == null) {
      return ResponseEntity.internalServerError().build();
    }
    return ResponseEntity.ok(response);
  }
}

