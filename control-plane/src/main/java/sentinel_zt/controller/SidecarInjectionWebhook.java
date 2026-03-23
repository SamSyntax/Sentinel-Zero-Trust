package sentinel_zt.controller;

import io.fabric8.kubernetes.api.model.Pod;
import io.fabric8.kubernetes.api.model.admission.v1.AdmissionResponse;
import io.fabric8.kubernetes.api.model.admission.v1.AdmissionReview;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import com.fasterxml.jackson.databind.ObjectMapper;

import java.util.Base64;
import java.util.Map;

@Slf4j
@RestController
@RequestMapping("/webhook")
public class SidecarInjectionWebhook {

    @Value("${spiffe.trust-domain:cluster.local}")
    private String trustDomain;

    @Value("${control-plane.service-name:sentinel-control-plane}")
    private String controlPlaneServiceName;

    @Value("${control-plane.namespace:sentinel-control-plane}")
    private String controlPlaneNamespace;

    @Value("${control-plane.port:8081}")
    private int controlPlanePort;

    @Value("${control-plane.grpc-port:9090}")
    private int controlPlaneGrpcPort;

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
            ObjectMapper mapper = new ObjectMapper();
            Pod pod = mapper.convertValue(admissionReview.getRequest().getObject(), Pod.class);
            Map<String, String> annotations = pod.getMetadata().getAnnotations();

            if (annotations != null && "true".equals(annotations.get("sentinel-zt.io/inject"))) {
                String podName = pod.getMetadata().getName() != null ? pod.getMetadata().getName() : 
                                 (pod.getMetadata().getGenerateName() != null ? pod.getMetadata().getGenerateName() : "unknown-pod");

                String serviceName = annotations.getOrDefault("sentinel-zt.io/service-name", podName);

                String namespace = pod.getMetadata().getNamespace() != null ? pod.getMetadata().getNamespace() : "default";
                String serviceAccount = pod.getSpec().getServiceAccountName() != null ? pod.getSpec().getServiceAccountName() : "default";

                String controlPlaneUrl = String.format("http://%s.%s.svc.cluster.local:%d", controlPlaneServiceName, controlPlaneNamespace, controlPlanePort);
                String targetGrpc = String.format("%s.%s.svc.cluster.local:%d", controlPlaneServiceName, controlPlaneNamespace, controlPlaneGrpcPort);

                log.info("Injecting sidecar into pod: {} (service: {}, namespace: {}, sa: {})", 
                        podName, serviceName, namespace, serviceAccount);

                String initContainer = "{" +
                        "\"name\": \"sentinel-init\", " +
                        "\"image\": \"kind.local/sentinel-init:latest\", " +
                        "\"imagePullPolicy\": \"Never\", " +
                        "\"securityContext\": {" +
                        "  \"capabilities\": {\"add\": [\"NET_ADMIN\", \"NET_RAW\"]}" +
                        "}, " +
                        "\"args\": [\"--inboundPort\", \"15006\", \"--outboundPort\", \"15001\"]" +
                        "}";

                String sidecarContainer = "{" +
                        "\"name\": \"sentinel-proxy\", " +
                        "\"image\": \"sentinel-data-plane:latest\", " +
                        "\"imagePullPolicy\": \"Never\", " +
                        "\"securityContext\": {" +
                        "  \"privileged\": true, " +
                        "  \"runAsUser\": 0, " +
                        "  \"capabilities\": {\"add\": [\"NET_ADMIN\"]}" +
                        "}, " +
                        "\"ports\": [" +
                        "  {\"containerPort\": 15006, \"name\": \"http-sentinel\"}," +
                        "  {\"containerPort\": 15001, \"name\": \"https-sentinel\"}" +
                        "], " +
                        "\"env\": [" +
                        "  {\"name\": \"SERVICE_NAME\", \"value\": \"" + serviceName + "\"}, " +
                        "  {\"name\": \"SPIFFE_TRUST_DOMAIN\", \"value\": \"" + trustDomain + "\"}, " +
                        "  {\"name\": \"CONTROL_PLANE_URL\", \"value\": \"" + controlPlaneUrl + "/api/v1/identity/issue\"}, " +
                        "  {\"name\": \"TARGET_GRPC\", \"value\": \"" + targetGrpc + "\"}, " +
                        "  {\"name\": \"PROXY_MODE\", \"value\": \"redirect\"}, " +
                        "  {\"name\": \"POD_NAME\", \"valueFrom\": {\"fieldRef\": {\"fieldPath\": \"metadata.name\"}}}, " +
                        "  {\"name\": \"POD_NAMESPACE\", \"valueFrom\": {\"fieldRef\": {\"fieldPath\": \"metadata.namespace\"}}}, " +
                        "  {\"name\": \"SERVICE_ACCOUNT\", \"valueFrom\": {\"fieldRef\": {\"fieldPath\": \"spec.serviceAccountName\"}}}, " +
                        "  {\"name\": \"CA_CERT_PATH\", \"value\": \"/etc/certs/root_ca.crt\"}, " +
                        "  {\"name\": \"POD_IP\", \"valueFrom\": {\"fieldRef\": {\"fieldPath\": \"status.podIP\"}}}" +
                        "], " +
                        "\"volumeMounts\": [" +
                        "  {\"name\": \"sentinel-certs\", \"mountPath\": \"/etc/certs\", \"readOnly\": true}" +
                        "]" +
                        "}";

                String volume = "{" +
                        "\"name\": \"sentinel-certs\", " +
                        "\"secret\": {\"secretName\": \"sentinel-root-ca\"}" +
                        "}";

                boolean hasInit = pod.getSpec().getInitContainers() != null && !pod.getSpec().getInitContainers().isEmpty();
                String initPath = hasInit ? "/spec/initContainers/-" : "/spec/initContainers";
                String initVal = hasInit ? initContainer : "[" + initContainer + "]";

                boolean hasVols = pod.getSpec().getVolumes() != null && !pod.getSpec().getVolumes().isEmpty();
                String volPath = hasVols ? "/spec/volumes/-" : "/spec/volumes";
                String volVal = hasVols ? volume : "[" + volume + "]";

                String patch = "[" +
                        "{\"op\": \"add\", \"path\": \"" + initPath + "\", \"value\": " + initVal + "}, " +
                        "{\"op\": \"add\", \"path\": \"/spec/containers/-\", \"value\": " + sidecarContainer + "}, " +
                        "{\"op\": \"add\", \"path\": \"" + volPath + "\", \"value\": " + volVal + "}" +
                        "]";

                response.setAllowed(true);
                response.setPatchType("JSONPatch");
                response.setPatch(Base64.getEncoder().encodeToString(patch.getBytes()));
                
                log.info("Sidecar injection patch applied for pod: {}", podName);
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
