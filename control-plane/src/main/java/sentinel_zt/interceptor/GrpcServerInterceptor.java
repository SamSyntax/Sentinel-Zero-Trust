package sentinel_zt.interceptor;

import org.springframework.stereotype.Component;
import org.jboss.logging.MDC;
import org.slf4j.Logger;

import io.grpc.Metadata;
import io.grpc.ServerCall;
import io.grpc.ServerCallHandler;
import io.grpc.ServerInterceptor;
import io.grpc.Status;
import sentinel_zt.service.AuditService;
import sentinel_zt.service.K8sTokenReviewService;

@Component
public class GrpcServerInterceptor implements ServerInterceptor {
  public static final Logger log = org.slf4j.LoggerFactory.getLogger(GrpcServerInterceptor.class);
  private static final Metadata.Key<String> TOKEN_KEY = Metadata.Key.of("x-sentinel-token", Metadata.ASCII_STRING_MARSHALLER);

  private final K8sTokenReviewService k8sService;
  private final AuditService auditService;

  public GrpcServerInterceptor(K8sTokenReviewService k8sService, AuditService auditService) {
    this.k8sService = k8sService;
    this.auditService = auditService;
  }

  @Override
public <ReqT, RespT> ServerCall.Listener<ReqT> interceptCall(ServerCall<ReqT, RespT> call, Metadata metadata, ServerCallHandler<ReqT, RespT> next) {
  String token = metadata.get(TOKEN_KEY);
  String clientIp = call.getAttributes().get(
      io.grpc.Grpc.TRANSPORT_ATTR_REMOTE_ADDR).toString();
  if(token == null || token.isBlank()) {
    log.warn("Missing gRPC token - method={}, clientIp={}", call.getMethodDescriptor().getFullMethodName(), clientIp);
    auditService.logUnauthorizedAccessAttempt(call.getMethodDescriptor().getFullMethodName(), clientIp, "gRPC");
    call.close(Status.UNAUTHENTICATED.withDescription("Missing token"), new Metadata());
    return new ServerCall.Listener<ReqT>() {};
  }

  if (token.startsWith("Bearer ")) {
    token = token.substring(7);
  }

  try {
    if (k8sService.validateToken(token)) {
      String serviceName = k8sService.getServiceAccountName(token);
      MDC.put("serviceIdentity", serviceName);
      MDC.put("traceId", java.util.UUID.randomUUID().toString());
      log.info("gRPC request authorized - serviceName={}, method={}, clientIp={}", serviceName, call.getMethodDescriptor().getFullMethodName(), clientIp);
      auditService.logAuthenticationAttempt(serviceName, true, null);

      return next.startCall(call, metadata);
    }
    log.error("gRPC token validation failed = method{}, clientIp={}", call.getMethodDescriptor().getFullMethodName(), clientIp);
    auditService.logAuthenticationAttempt("unknown", false, "invalid token");
    call.close(Status.UNAUTHENTICATED.withDescription("Invalid token"), new Metadata());
  } catch (Exception e) {
    log.error("gRPC token validation error= method{}, error={}", call.getMethodDescriptor().getFullMethodName(), e.getMessage());
    auditService.logAuthenticationAttempt("unknown", false, "validation_error: " + e.getMessage());
    call.close(Status.UNAUTHENTICATED.withDescription("Validation error"), new Metadata());
  }

  return new ServerCall.Listener<ReqT>() {};

}

}
