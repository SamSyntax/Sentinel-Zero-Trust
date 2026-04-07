import { faker } from '@faker-js/faker'
import { usersTable } from './db/schema/userSchema'
import { db } from './db/client';


type DbUser = typeof usersTable.$inferInsert;
export function generateRandomUser(): DbUser {
  return {
    email: faker.internet.email(),
    name: faker.person.fullName(),
    age: faker.number.int({ min: 18, max: 100 })
  }
}

async function seedUsers() {
  try {
    for (let i = 0; i < 100; i++) {
      console.log("Iteration " + i);
      await db.insert(usersTable).values(generateRandomUser());
    }
  } catch (err: any) {
    console.error(err);
  }
}

await seedUsers();
process.exit(0);
