package sentinel_zt.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;
import lombok.extern.slf4j.Slf4j;

@Slf4j
@Builder
@Data
@NoArgsConstructor
@AllArgsConstructor
public class IdentityResponse {
  private String certificate;
  private String privateKey;
  private String issuingCa;
  private String serialNumber;
}
