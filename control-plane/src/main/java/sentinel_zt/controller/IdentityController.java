package sentinel_zt.controller;

import sentinel_zt.dto.IdentityResponse;
import sentinel_zt.dto.IdentityRequest;
import sentinel_zt.service.VaultPkiService;
import jakarta.validation.Valid;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/v1/identity")
public class IdentityController {
  private final VaultPkiService pkiService;

  public IdentityController(VaultPkiService pkiService) {
    this.pkiService = pkiService;
  }

  @PostMapping("/issue")
  public ResponseEntity<IdentityResponse> issueIdentity(@Valid @RequestBody IdentityRequest request) {
    IdentityResponse response = pkiService.issueCertificate(request.getServiceName());
    return ResponseEntity.ok(response);

  }

}
