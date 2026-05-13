const PORT = parseInt(Bun.env.PORT || "3005", 10);
const HOST = Bun.env.HOST || "0.0.0.0";
const ECHO_SERVICE_URL =
  Bun.env.ECHO_SERVICE_URL ||
  "http://echo-service.echo-service.svc.cluster.local/api/health";
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

async function checkEchoService() {
  const startedAt = new Date().toISOString();

  try {
    const response = await fetch(ECHO_SERVICE_URL, {
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
        message: "echo-service check completed",
        echoServiceUrl: ECHO_SERVICE_URL,
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
        message: "echo-service check failed",
        echoServiceUrl: ECHO_SERVICE_URL,
        error: message,
        at: startedAt,
      }),
    );
  }
}

setInterval(() => {
  void checkEchoService();
}, LOOP_INTERVAL_MS);

void checkEchoService();

type User = {
  id: string;
  name: string;
  age: number;
};

const users: User[] = [
  { id: "1", name: "Jan", age: 30 },
  { id: "2", name: "Klaudia", age: 25 },
  { id: "3", name: "Michał", age: 35 },
  { id: "4", name: "Rafał", age: 40 },
  { id: "5", name: "Ola", age: 45 },
];
const getUser = () => {
  return users[Math.floor(Math.random() * users.length)];
}

Bun.serve({
  hostname: HOST,
  port: PORT,
  fetch(request) {
    const url = new URL(request.url);

    if (url.pathname === "/api/health") {
      return Response.json({ status: "ok", service: "users-service" });
    }

    if (url.pathname === "/api/users") {
      return Response.json({ status: "ok", user: getUser(), service: "users-service", now: new Date().toISOString() });
    }

    if (url.pathname === "/api/last-check") {
      return Response.json(lastCheck);
    }

    return Response.json(
      {
        service: "echo-service",
        path: url.pathname,
        echoServiceUrl: ECHO_SERVICE_URL,
        lastCheck,
      },
      { status: 200 },
    );
  },
});

console.log(
  JSON.stringify({
    level: "info",
    message: "users-service started",
    host: HOST,
    port: PORT,
    echoServiceUrl: ECHO_SERVICE_URL,
    loopIntervalMs: LOOP_INTERVAL_MS,
  }),
);
