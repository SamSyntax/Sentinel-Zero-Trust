package sentinel_zt.dto;

import jakarta.validation.constraints.NotBlank;
import lombok.Data;
import lombok.extern.slf4j.Slf4j;

@Slf4j
@Data
public class IdentityRequest {
  @NotBlank(message = "Service name is required")
  private String serviceName;
}
