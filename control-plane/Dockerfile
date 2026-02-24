FROM maven:3.9-eclipse-temurin-21-alpine AS builder
WORKDIR /build

COPY pom.xml .
RUN mvn dependency:go-offline

COPY src/ src/
RUN mvn clean package -DskipTests

FROM eclipse-temurin:21-jre-alpine

RUN addgroup -S sentinel && adduser -S sentinel -G sentinel

WORKDIR /app
COPY --from=builder /build/target/*.jar app.jar

USER sentinel
EXPOSE 8081

ENTRYPOINT [ "java", "-jar", "app.jar" ]
