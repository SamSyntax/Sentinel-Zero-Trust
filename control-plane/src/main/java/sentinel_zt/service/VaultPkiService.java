package sentinel_zt.service;

import java.util.Map;

import org.springframework.stereotype.Service;
import org.springframework.vault.core.VaultTemplate;
import org.springframework.vault.support.VaultResponse;

import sentinel_zt.dto.IdentityResponse;

@Service
public class VaultPkiService {

  private final VaultTemplate vaultTemplate;

  public VaultPkiService(VaultTemplate vaultTemplate) {
    this.vaultTemplate = vaultTemplate;
  }

  public IdentityResponse issueCertificate(String serviceName) {
    Map<String,Object> request = Map.of(
        "common_name", serviceName + ".sentinel.local",
        "ttl", "30m"
        );
    VaultResponse response = vaultTemplate.write("pki/issue/sentinel-service", request);
    Map<String, Object> data = response.getData();



    return IdentityResponse.builder()
        .certificate((String) data.get("certificate"))
        .privateKey((String) data.get("private_key"))
        .issuingCa((String) data.get("issuing_ca"))
        .serialNumber((String) data.get("serial_number"))
        .build();
  }

  public VaultTemplate getVaultTemplate() {
	return vaultTemplate;
  }
}
