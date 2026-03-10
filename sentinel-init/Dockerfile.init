FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o init-container cmd/main.go

FROM alpine:3.19
RUN apk add --no-cache iptables iproute2

WORKDIR /app
COPY --from=builder /app/init-container .

RUN chmod +x ./init-container

ENTRYPOINT ["./init-container"]
