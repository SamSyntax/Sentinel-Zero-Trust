FROM maven:3.9-eclipse-temurin-21 AS builder
# RUN apk add --no-cache gcompat

WORKDIR /build

COPY pom.xml .
RUN mvn dependency:go-offline || true

COPY src/ src/

RUN mvn clean package -DskipTests -X -q

FROM eclipse-temurin:21-jre-alpine

RUN addgroup -S sentinel && adduser -S sentinel -G sentinel

WORKDIR /app
COPY --from=builder /build/target/*.jar app.jar

USER sentinel
EXPOSE 8081
EXPOSE 9090

ENTRYPOINT [ "java", "-jar", "app.jar" ]
