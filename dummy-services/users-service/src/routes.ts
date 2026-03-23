import type { FastifyInstance, FastifyPluginAsync } from "fastify";
import { createUser, deleteUser, getSingleUser, getUsers, updateUser } from "./handlers/users";

const userRoutes: FastifyPluginAsync = async (fastify: FastifyInstance) => {
  fastify.get("/users", getUsers);
  fastify.get("/users/:id", getSingleUser);
  fastify.post("/users", createUser);
  fastify.put("/users/:id", updateUser);
  fastify.delete("/users/:id", deleteUser);
}

export const apiRoutes: FastifyPluginAsync = async (fastify: FastifyInstance) => {
  fastify.register(userRoutes, { prefix: "/v1" });
}
