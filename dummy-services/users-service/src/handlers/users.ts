import type { FastifyReply, FastifyRequest } from "fastify";
import type { IdParam, User } from "../schemas/user";


const users: User[] = [
  { id: 1, name: "John", email: "john@example.com" },
  { id: 2, name: "Jane", email: "jane@example.com" },
  { id: 3, name: "Bob", email: "bob@example.com" },
  { id: 4, name: "Alice", email: "alice@example.com" },
  { id: 5, name: "Charlie", email: "charlie@example.com" },
  { id: 6, name: "Dave", email: "dave@example.com" },
  { id: 7, name: "Eve", email: "eve@example.com" },
  { id: 8, name: "Frank", email: "frank@example.com" },
];


export function getUsers(req: FastifyRequest, reply: FastifyReply) {
  req.log.info("getUsers hit");
  reply.send(users).status(200);
}

export function getSingleUser(req: FastifyRequest<{ Params: IdParam }>, reply: FastifyReply) {
  req.log.info("getSingleUser hit");
  const { id } = req.params;
  const user = users.find((user) => user.id === Number(id));
  if (!user) {
    return reply.status(404).send({ message: "User not found" });
  }
  reply.send(user).status(200);
}

export function createUser(req: FastifyRequest<{ Body: User }>, reply: FastifyReply) {
  try {
    const { email, name } = req.body;
    const user: User = {
      id: users.length + 1,
      name,
      email
    }
    users.push(user);
    reply.send(user).status(201);
  } catch (err: any) {
    reply.status(400).send({ message: err.message });
  }
}
