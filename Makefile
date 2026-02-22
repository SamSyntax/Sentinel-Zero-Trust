vault-reset:
	docker compose -f infra/docker-compose.yml down && docker compose -f infra/docker-compose.yml up -d

build:
	go build -o data-plane/bin/plane data-plane/cmd/main.go  && data-plane/bin/plane
