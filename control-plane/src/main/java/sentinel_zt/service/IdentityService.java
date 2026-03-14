package sentinel_zt.service;

import org.springframework.grpc.server.service.GrpcService;

import io.grpc.Status;
import io.grpc.stub.StreamObserver;
import sentinel_zt.CertificateRequest;
import sentinel_zt.CertificateResponse;
import sentinel_zt.dto.IdentityResponse;
import sentinel_zt.CertificateIssuerServiceGrpc.CertificateIssuerServiceImplBase;

@GrpcService
public class IdentityService extends CertificateIssuerServiceImplBase {
  private final VaultPkiService pkiService;

  public IdentityService(VaultPkiService pkiService) {
    this.pkiService = pkiService;
  }
  @Override
  public void getCertificate(CertificateRequest request, StreamObserver<CertificateResponse> responseObserver) {
    String serviceName = request.getServiceName();
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
