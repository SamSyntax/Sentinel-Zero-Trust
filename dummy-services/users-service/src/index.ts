import Fastify from "fastify";
import { apiRoutes } from "./routes";

const fastify = Fastify({
  logger: true
});


const PORT = parseInt(Bun.env.PORT || "3005");
const HOST = Bun.env.HOST || "0.0.0.0";

async function start() {
  try {
    await fastify.register(apiRoutes, { prefix: "/api" });
    fastify.listen({
      host: HOST,
      port: PORT,
    })
    fastify.log.info(`Server listening on ${HOST}:${PORT}`);
  } catch (err: any) {
    fastify.log.error(`Somrthing wen wrong with the fastify server: ${err.message}`)
    process.exit(1);
  }
}

start();


