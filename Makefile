vault-reset:
	docker compose -f infra/docker-compose.yml down && docker compose -f infra/docker-compose.yml up -d
