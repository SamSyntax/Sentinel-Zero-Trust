package sentinel_zt.service;

import org.springframework.stereotype.Service;
import org.springframework.vault.core.VaultPkiOperations;
import org.springframework.vault.core.VaultTemplate;
import org.springframework.vault.support.VaultCertificateRequest;
import org.springframework.vault.support.VaultCertificateResponse;
import sentinel_zt.dto.IdentityResponse;

@Service
public class VaultPkiService {

  private final VaultTemplate vaultTemplate;

  public VaultPkiService(VaultTemplate vaultTemplate) {
    this.vaultTemplate = vaultTemplate;
  }

  public IdentityResponse issueCertificate(String serviceName) {
    VaultPkiOperations pkiOps = vaultTemplate.opsForPki("pki");

    VaultCertificateRequest request = VaultCertificateRequest.builder()
        .commonName(serviceName + ".sentinel.local")
        .ttl(java.time.Duration.ofHours(72))
        .build();

    VaultCertificateResponse response = pkiOps.issueCertificate(
        "sentinel-service",
        request);


    return IdentityResponse.builder()
        .certificate(response.getData().getCertificate())
        .privateKey(response.getData().getPrivateKey())
        .issuingCa(response.getData().getIssuingCaCertificate())
        .serialNumber(response.getData().getSerialNumber())
        .build();
  }

  public VaultTemplate getVaultTemplate() {
	return vaultTemplate;
  }
}
