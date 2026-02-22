DATA_PLANE_BINARY_NAME=plane
DATA_PLANE_BUILD_DIR=data-plane/bin

.PHONY: all build clean run

all: build

vault-reset:
	docker compose -f infra/docker-compose.yml down && docker compose -f infra/docker-compose.yml up -d

build:
	@mkdir -p $(DATA_PLANE_BUILD_DIR)
	go build -o $(DATA_PLANE_BUILD_DIR)/$(DATA_PLANE_BINARY_NAME) ./data-plane/cmd

run: build
	./$(DATA_PLANE_BUILD_DIR)/$(DATA_PLANE_BINARY_NAME)

clean:
	rm -rf $(DATA_PLANE_BUILD_DIR)

