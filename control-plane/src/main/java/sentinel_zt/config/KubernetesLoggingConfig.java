package sentinel_zt.config;

import jakarta.annotation.PostConstruct;
import lombok.extern.slf4j.Slf4j;
import org.slf4j.MDC;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.net.InetAddress;
import java.net.UnknownHostException;
import java.util.UUID;

@Component
@Slf4j
public class KubernetesLoggingConfig {

    @Value("${spring.application.name:sentinel-control-plane}")
    private String applicationName;

    @Value("${logging.environment:production}")
    private String environment;

    @Value("${HOSTNAME:unknown}")
    private String podName;

    @Value("${POD_NAMESPACE:default}")
    private String namespace;

    @Value("${CONTAINER_NAME:unknown}")
    private String containerName;

    @PostConstruct
    public void init() {
        MDC.put("app_name", applicationName);
        MDC.put("environment", environment);
        MDC.put("pod_name", podName);
        MDC.put("namespace", namespace);
        MDC.put("container_name", containerName);
        
        try {
            InetAddress ip = InetAddress.getLocalHost();
            MDC.put("host_ip", ip.getHostAddress());
            MDC.put("hostname", ip.getHostName());
        } catch (UnknownHostException e) {
            MDC.put("host_ip", "unknown");
            MDC.put("hostname", "unknown");
        }

        log.info("Kubernetes logging context initialized: app={}, namespace={}, pod={}, env={}", 
                 applicationName, namespace, podName, environment);
    }

    public static void addRequestContext(String traceId, String serviceIdentity) {
        if (traceId != null) {
            MDC.put("trace_id", traceId);
        }
        if (serviceIdentity != null) {
            MDC.put("service_identity", serviceIdentity);
        }
        MDC.put("request_id", UUID.randomUUID().toString());
    }

    public static void clearRequestContext() {
        MDC.remove("trace_id");
        MDC.remove("service_identity");
        MDC.remove("request_id");
    }
}
