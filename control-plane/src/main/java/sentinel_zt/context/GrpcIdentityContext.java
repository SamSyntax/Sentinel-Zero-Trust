package sentinel_zt.context;

import io.grpc.Context;

public class GrpcIdentityContext {
  public static final Context.Key<String> IDENTITY_KEY = Context.key("service-identity");
  public static final Context.Key<String> POD_UID_KEY = Context.key("pod-uid");
  public static final Context.Key<String> POD_NAME_KEY = Context.key("pod-name");

  public static Context withIdentity(String identity, String podUid, String podName) {
    Context ctx = Context.current().withValue(IDENTITY_KEY, identity);
    ctx = ctx.withValue(POD_UID_KEY, podUid);
    ctx = ctx.withValue(POD_NAME_KEY, podName);
    return ctx;
  }
  public static String getIdentity() {
    return IDENTITY_KEY.get(Context.current());
  }
  public static String getPodUid() {
    return POD_UID_KEY.get(Context.current());
  }
  public static String getPodName() {
    return POD_NAME_KEY.get(Context.current());
  }
}
