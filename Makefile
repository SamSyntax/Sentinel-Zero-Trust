DATA_PLANE_BINARY_NAME=plane
DATA_PLANE_BUILD_DIR=data-plane/bin

.PHONY: all build clean run

all: build

vault-reset:
	docker compose -f infra/docker-compose.yml down && docker compose -f infra/docker-compose.yml up -d

build:
	@mkdir -p $(DATA_PLANE_BUILD_DIR)
	go build -o $(DATA_PLANE_BUILD_DIR)/$(DATA_PLANE_BINARY_NAME) ./data-plane/cmd

run-data-plane: build
	./$(DATA_PLANE_BUILD_DIR)/$(DATA_PLANE_BINARY_NAME)

run-control-plane:
	cd control-plane && mvn spring-boot:run

run: run-data-plane run-control-plane

clean:
	rm -rf $(DATA_PLANE_BUILD_DIR)

docker:
	docker compose -f infra/docker-compose.yml down && \
	docker compose -f infra/docker-compose.yml up -d && \
	infra/vault/init-pki.sh && infra/vault/init-proxyclient-certs.sh

recreate-cluster:
	infra/k8s/recreate-cluster.sh

build-control-plane-image:
	docker build -t "sentinel-control-plane:latest" control-plane/
	kind load docker-image sentinel-control-plane:latest --name sentinel-zt
	kubectl rollout restart deployment/sentinel-control-plane

build-data-plane-image:
	docker build -t "sentinel-data-plane:latest" data-plane/
	kind load docker-image sentinel-data-plane:latest --name sentinel-zt
	kubectl rollout restart deployment/sentinel-data-plane

build-images: build-control-plane-image build-data-plane-image

reload-pod-images: build-images


