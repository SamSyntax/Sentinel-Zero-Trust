package sentinel_zt.context;

import io.grpc.Context;

public class GrpcIdentityContext {
  public static final Context.Key<String> IDENTITY_KEY = Context.key("service-identity");
  public static Context withIdentity(String identity) {
    return Context.current().withValue(IDENTITY_KEY, identity);
  }
  public static String getIdentity() {
    return IDENTITY_KEY.get(Context.current());
  }
}
