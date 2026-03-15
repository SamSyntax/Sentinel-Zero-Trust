package sentinel_zt.service;

import org.slf4j.LoggerFactory;
import org.slf4j.Logger;
import org.springframework.grpc.server.service.GrpcService;

import io.grpc.Status;
import io.grpc.stub.StreamObserver;
import sentinel_zt.CertificateRequest;
import sentinel_zt.CertificateResponse;
import sentinel_zt.dto.IdentityResponse;
import sentinel_zt.interceptor.GrpcServerInterceptor;
import sentinel_zt.CertificateIssuerServiceGrpc.CertificateIssuerServiceImplBase;
import sentinel_zt.context.GrpcIdentityContext;

@GrpcService(interceptors = {GrpcServerInterceptor.class})
public class IdentityService extends CertificateIssuerServiceImplBase {
  private final VaultPkiService pkiService;
  private static final Logger log = LoggerFactory.getLogger(IdentityService.class);

  public IdentityService(VaultPkiService pkiService) {
    this.pkiService = pkiService;
  }
  @Override
  public void getCertificate(CertificateRequest request, StreamObserver<CertificateResponse> responseObserver) {
    String callerIdentity = GrpcIdentityContext.getIdentity();
    String serviceName = request.getServiceName();
    log.debug("Received request for service {}", serviceName);
    if(callerIdentity == null || !callerIdentity.equals(serviceName)) {
      log.warn("Identity spoofing attempt = caller={}, requested={}", callerIdentity, serviceName);
      responseObserver.onError(Status.PERMISSION_DENIED.withDescription("Cannot request certificate for different service").asRuntimeException());
      return;
    }
    IdentityResponse response = pkiService.issueCertificate(serviceName);
    try {
      if(response == null) {
        responseObserver.onError(Status.INTERNAL.withDescription("Certificate response is empty").asRuntimeException());
        return;
      }
      var grpcResponse = CertificateResponse.newBuilder()
        .setCertificate(response.getCertificate())
        .setPrivateKey(response.getPrivateKey())
        .setIssuingCa(response.getIssuingCa())
        .setSerialNumber(response.getSerialNumber())
        .build();
      responseObserver.onNext(grpcResponse);
      responseObserver.onCompleted();
    } catch (Exception e) {
      responseObserver.onError(Status.INTERNAL.withDescription("Certificate issuance failed: " + e.getMessage()).asRuntimeException());
    }
  }
}
