import type { FastifyReply, FastifyRequest } from "fastify";
import type { IdParam, User } from "../schemas/user";
import { db } from "../db/client";
import { usersTable } from "../db/schema/schema";
import { eq } from "drizzle-orm";


export async function getUsers(_: FastifyRequest, reply: FastifyReply) {
  const users = await db.query.usersTable.findMany();
  reply.send(users).status(200);
}

export async function getSingleUser(req: FastifyRequest<{ Params: IdParam }>, reply: FastifyReply) {
  const { id } = req.params;

  const user = await db.query.usersTable.findFirst({
    where: eq(usersTable.id, Number(id))
  })

  if (!user) {
    return reply.status(404).send({ message: "User not found" });
  }
  reply.send(user).status(200);
}

export async function createUser(req: FastifyRequest<{ Body: User }>, reply: FastifyReply) {
  try {
    const { email, name, age } = req.body;
    console.log(email, name, age)
    const user = await db.insert(usersTable).values({
      email,
      name,
      age
    }).returning();

    reply.send(user).status(201);
  } catch (err: any) {
    reply.status(400).send({ message: err.message });
  }
}

export async function updateUser(req: FastifyRequest<{ Body: User, Params: IdParam }>, reply: FastifyReply) {
  try {
    const { email, name, age } = req.body;
    console.log(email, name, age)
    const user = await db.update(usersTable).set({
      email,
      name,
      age
    }).where(eq(usersTable.id, Number(req.params.id)))

    reply.send(user).status(204);
  }

  catch (err: any) {
    reply.status(400).send({ message: err.message });
  }
}

export async function deleteUser(req: FastifyRequest<{ Params: IdParam }>, reply: FastifyReply) {
  try {
    const user = await db.delete(usersTable).where(eq(usersTable.id, Number(req.params.id))).returning();
    if (!user || user.length === 0) {
      return reply.status(204).send({ message: "User not found" });
    }
    reply.send().status(200)

  } catch (err: any) {
    reply.status(500).send({ message: err.message });
  }
}
