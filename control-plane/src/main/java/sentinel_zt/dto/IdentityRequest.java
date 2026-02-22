package sentinel_zt.dto;

import jakarta.validation.constraints.NotBlank;
import lombok.Data;

@Data
public class IdentityRequest {
  @NotBlank(message = "Service name is required")
  private String serviceName;

}
