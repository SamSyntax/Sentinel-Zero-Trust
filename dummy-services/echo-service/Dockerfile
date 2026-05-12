FROM oven/bun:1-alpine
WORKDIR /usr/src/app

COPY package.json ./
COPY src ./src

USER bun
EXPOSE 3010/tcp
ENTRYPOINT ["bun", "run", "src/index.ts"]
