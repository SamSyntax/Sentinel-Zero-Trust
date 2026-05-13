const PORT = parseInt(Bun.env.PORT || "3010", 10);
const HOST = Bun.env.HOST || "0.0.0.0";
const USERS_SERVICE_URL =
  Bun.env.USERS_SERVICE_URL ||
  "http://users-service.users-service.svc.cluster.local/api/users";
const LOOP_INTERVAL_MS = parseInt(Bun.env.LOOP_INTERVAL_MS || "15000", 10);

type LastCheck = {
  at: string;
  ok: boolean;
  status?: number;
  body?: string;
  error?: string;
};

let lastCheck: LastCheck = {
  at: new Date().toISOString(),
  ok: false,
  error: "loop not started yet",
};

async function checkUsersService() {
  const startedAt = new Date().toISOString();

  try {
    const response = await fetch(USERS_SERVICE_URL, {
      headers: {
        "x-echo-service": "periodic-check",
      },
    });
    const body = await response.text();

    lastCheck = {
      at: startedAt,
      ok: response.ok,
      status: response.status,
      body,
    };

    console.log(
      JSON.stringify({
        level: response.ok ? "info" : "warn",
        message: "users-service check completed",
        usersServiceUrl: USERS_SERVICE_URL,
        status: response.status,
        ok: response.ok,
        at: startedAt,
      }),
    );
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    lastCheck = {
      at: startedAt,
      ok: false,
      error: message,
    };

    console.log(
      JSON.stringify({
        level: "error",
        message: "users-service check failed",
        usersServiceUrl: USERS_SERVICE_URL,
        error: message,
        at: startedAt,
      }),
    );
  }
}

setInterval(() => {
  void checkUsersService();
}, LOOP_INTERVAL_MS);

Bun.serve({
  hostname: HOST,
  port: PORT,
  fetch(request) {
    const url = new URL(request.url);

    if (url.pathname === "/api/health") {
      return Response.json({ status: "ok", service: "echo-service" });
    }

    if (url.pathname === "/api/ping") {
      return Response.json({ status: "ok", service: "echo-service", now: new Date().toISOString() });
    }

    if (url.pathname === "/api/last-check") {
      return Response.json(lastCheck);
    }

    return Response.json(
      {
        service: "echo-service",
        path: url.pathname,
        usersServiceUrl: USERS_SERVICE_URL,
        lastCheck,
      },
      { status: 200 },
    );
  },
});

console.log(
  JSON.stringify({
    level: "info",
    message: "echo-service started",
    host: HOST,
    port: PORT,
    usersServiceUrl: USERS_SERVICE_URL,
    loopIntervalMs: LOOP_INTERVAL_MS,
  }),
);
