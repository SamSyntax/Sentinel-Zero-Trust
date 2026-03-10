package sentinel_zt.controller;

import io.fabric8.kubernetes.api.model.Pod;
import io.fabric8.kubernetes.api.model.admission.v1.AdmissionResponse;
import io.fabric8.kubernetes.api.model.admission.v1.AdmissionReview;
import lombok.extern.slf4j.Slf4j;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.Base64;
import java.util.Map;

@Slf4j
@RestController
@RequestMapping("/webhook")
public class SidecarInjectionWebhook {
    @PostMapping("/mutate")
    public AdmissionReview mutate(@RequestBody AdmissionReview admissionReview) {
        if (admissionReview.getRequest() == null) {
            log.warn("Received empty admission review request");
            return admissionReview;
        }

        String uid = admissionReview.getRequest().getUid();
        log.info("Received admission review request: {}", uid);

        AdmissionResponse response = new AdmissionResponse();
        response.setUid(uid);

        try {
            Pod pod = (Pod) admissionReview.getRequest().getObject();
            Map<String, String> annotations = pod.getMetadata().getAnnotations();

            if (annotations != null && "true".equals(annotations.get("sentinel-zt.io/inject"))) {
                String serviceName = annotations.getOrDefault("sentinel-zt.io/service-name",
                        pod.getMetadata().getName() != null ? pod.getMetadata().getName() : "unknown-service");

                log.info("Injecting sidecar into pod: {} (service: {})", pod.getMetadata().getName(), serviceName);

                String patch = "[" +
                        "{\"op\": \"add\", \"path\": \"/spec/containers/-\", \"value\": {" +
                        "\"name\": \"sentinel-proxy\", " +
                        "\"image\": \"sentinel-data-plane:latest\", " +
                        "\"imagePullPolicy\": \"Never\", " +
                        "\"env\": [" +
                        "{\"name\": \"SERVICE_NAME\", \"value\": \"" + serviceName + "\"}, " +
                        "{\"name\": \"CONTROL_PLANE_URL\", \"value\": \"http://sentinel-control-plane.sentinel-control-plane.svc.cluster.local:8081/api/v1/identity/issue\"}, "
                        +
                        "{\"name\": \"CA_CERT_PATH\", \"value\": \"/etc/certs/root_ca.crt\"}" +
                        "], " +
                        "\"volumeMounts\": [{\"name\": \"sentinel-certs\", \"mountPath\": \"/etc/certs\", \"readOnly\": true}]"
                        +
                        "}}, " +
                        "{\"op\": \"add\", \"path\": \"/spec/volumes/-\", \"value\": {" +
                        "\"name\": \"sentinel-certs\", \"secret\": {\"secretName\": \"sentinel-root-ca\"}" +
                        "}}" +
                        "]";

                response.setAllowed(true);
                response.setPatchType("JSONPatch");
                response.setPatch(Base64.getEncoder().encodeToString(patch.getBytes()));
            } else {
                response.setAllowed(true);
            }
        } catch (Exception e) {
            log.error("Error during sidecar injection: {}", e.getMessage(), e);
            response.setAllowed(true);
        }

        admissionReview.setResponse(response);
        return admissionReview;
    }
}
