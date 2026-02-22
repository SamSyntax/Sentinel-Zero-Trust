package sentinel_zt.dto;

import lombok.Builder;
import lombok.Data;

@Builder
@Data
public class IdentityResponse {
  private String certificate;
  private String privateKey;
  private String issuingCa;
  private String serialNumber;

  public IdentityResponse(String certificate, String privateKey, String issuingCa, String serialNumber) {
    this.certificate = certificate;
    this.privateKey = privateKey;
    this.issuingCa = issuingCa;
    this.serialNumber = serialNumber;
  }

}
