package sentinel_zt.config;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import io.fabric8.kubernetes.client.KubernetesClient;
import io.fabric8.kubernetes.client.KubernetesClientBuilder;

@Configuration
public class KubernetesConfig {
  @Bean
  public KubernetesClient kubernetesClient() {
    KubernetesClient client = new KubernetesClientBuilder().build();
    return client;
  }
}
