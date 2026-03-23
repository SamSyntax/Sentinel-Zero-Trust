import type { FastifyInstance, FastifyPluginAsync } from "fastify";
import { createUser, getSingleUser, getUsers } from "./handlers/users";

const userRoutes: FastifyPluginAsync = async (fastify: FastifyInstance) => {
  fastify.get("/users", getUsers);
  fastify.get("/users/:id", getSingleUser);
  fastify.post("/users", createUser);
}

export const apiRoutes: FastifyPluginAsync = async (fastify: FastifyInstance) => {
  fastify.register(userRoutes, { prefix: "/v1" });
}
