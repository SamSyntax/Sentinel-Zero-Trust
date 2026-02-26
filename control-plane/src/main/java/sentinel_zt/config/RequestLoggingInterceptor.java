package sentinel_zt.config;

import lombok.extern.slf4j.Slf4j;
import org.slf4j.MDC;
import org.springframework.stereotype.Component;
import org.springframework.web.servlet.HandlerInterceptor;

import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import java.util.UUID;

@Component
@Slf4j
public class RequestLoggingInterceptor implements HandlerInterceptor {

    private static final String START_TIME_ATTR = "requestStartTime";
    private static final String TRACE_ID_HEADER = "X-Trace-ID";
    private static final String REQUEST_ID_HEADER = "X-Request-ID";

    @Override
    public boolean preHandle(HttpServletRequest request, HttpServletResponse response, Object handler) {
        String traceId = request.getHeader(TRACE_ID_HEADER);
        if (traceId == null || traceId.isEmpty()) {
            traceId = UUID.randomUUID().toString();
        }
        
        String requestId = request.getHeader(REQUEST_ID_HEADER);
        if (requestId == null || requestId.isEmpty()) {
            requestId = UUID.randomUUID().toString();
        }

        MDC.put("trace_id", traceId);
        MDC.put("request_id", requestId);
        MDC.put("http_method", request.getMethod());
        MDC.put("http_uri", request.getRequestURI());
        MDC.put("http_remote_addr", getClientIpAddress(request));
        MDC.put("http_user_agent", request.getHeader("User-Agent"));

        request.setAttribute(START_TIME_ATTR, System.currentTimeMillis());

        log.info("Incoming request: method={}, uri={}, queryString={}, traceId={}, requestId={}, clientIp={}",
                request.getMethod(),
                request.getRequestURI(),
                request.getQueryString(),
                traceId,
                requestId,
                getClientIpAddress(request));

        response.setHeader(TRACE_ID_HEADER, traceId);
        response.setHeader(REQUEST_ID_HEADER, requestId);

        return true;
    }

    @Override
    public void afterCompletion(HttpServletRequest request, HttpServletResponse response, Object handler, Exception ex) {
        Long startTime = (Long) request.getAttribute(START_TIME_ATTR);
        long duration = startTime != null ? System.currentTimeMillis() - startTime : 0;

        String traceId = MDC.get("trace_id");
        String requestId = MDC.get("request_id");

        if (ex != null) {
            log.error("Request failed: method={}, uri={}, status={}, durationMs={}, traceId={}, requestId={}, error={}",
                    request.getMethod(),
                    request.getRequestURI(),
                    response.getStatus(),
                    duration,
                    traceId,
                    requestId,
                    ex.getMessage(),
                    ex);
        } else {
            log.info("Request completed: method={}, uri={}, status={}, durationMs={}, traceId={}, requestId={}",
                    request.getMethod(),
                    request.getRequestURI(),
                    response.getStatus(),
                    duration,
                    traceId,
                    requestId);
        }

        log.debug("Request metrics: method={}, uri={}, status={}, durationMs={}, traceId={}, requestId={}, userAgent={}",
                request.getMethod(),
                request.getRequestURI(),
                response.getStatus(),
                duration,
                traceId,
                requestId,
                request.getHeader("User-Agent"));

        clearMdcContext();
    }

    private String getClientIpAddress(HttpServletRequest request) {
        String xForwardedFor = request.getHeader("X-Forwarded-For");
        if (xForwardedFor != null && !xForwardedFor.isEmpty()) {
            return xForwardedFor.split(",")[0].trim();
        }
        String xRealIp = request.getHeader("X-Real-IP");
        if (xRealIp != null && !xRealIp.isEmpty()) {
            return xRealIp;
        }
        return request.getRemoteAddr();
    }

    private void clearMdcContext() {
        MDC.remove("trace_id");
        MDC.remove("request_id");
        MDC.remove("http_method");
        MDC.remove("http_uri");
        MDC.remove("http_remote_addr");
        MDC.remove("http_user_agent");
    }
}
