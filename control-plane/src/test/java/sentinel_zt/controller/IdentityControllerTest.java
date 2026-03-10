package sentinel_zt.controller;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.webmvc.test.autoconfigure.WebMvcTest;
import org.springframework.http.MediaType;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import io.fabric8.kubernetes.client.KubernetesClient;

import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;

import sentinel_zt.dto.IdentityResponse;
import sentinel_zt.service.AuditService;
import sentinel_zt.service.K8sTokenReviewService;
import sentinel_zt.service.VaultPkiService;

@WebMvcTest(IdentityController.class)
class IdentityControllerTest {
  @Autowired
  private MockMvc mockMvc;

  @MockitoBean
  private VaultPkiService pkiService;
  @MockitoBean
  private AuditService auditService;
  @MockitoBean
  private K8sTokenReviewService k8sService;
  @MockitoBean
  private KubernetesClient kubernetesClient;

  @Test
  void issueIdentity_success_returns200WithCert() throws Exception {
    IdentityResponse response = IdentityResponse.builder()
      .certificate("cert")
      .privateKey("key")
      .serialNumber("1234")
      .build();
    when(pkiService.issueCertificate("test-service")).thenReturn(response);

    mockMvc.perform(post("/api/v1/identity/issue")
        .contentType(MediaType.APPLICATION_JSON)
        .header("X-Sentinel-Token", "Bearer valid-token")
        .content("""
          {"serviceName": "test-service"}
          """))
      .andExpect(status().isOk())
      .andExpect(jsonPath("$.certificate").value("cert"))
      .andExpect(jsonPath("$.serialNumber").value("1234"));
  }
}
