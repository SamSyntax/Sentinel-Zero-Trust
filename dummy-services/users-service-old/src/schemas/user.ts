import { Type, type Static } from "@sinclair/typebox";

export const User = Type.Object({
  name: Type.String(),
  email: Type.String(),
  age: Type.Number(),
});

export type User = Static<typeof User>;


export const IdParam = Type.Object({
  id: Type.String({
    pattern: '^[0-9a-fA-F]{24}$',
    description: 'The unique identifier for the user'
  })
})

export type IdParam = Static<typeof IdParam>;

