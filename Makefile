DATA_PLANE_BINARY_NAME=plane
DATA_PLANE_BUILD_DIR=data-plane

.PHONY: all build clean run

all: build

vault-reset:
	docker compose -f infra/docker-compose.yml down && docker compose -f infra/docker-compose.yml up -d

build:
	@mkdir -p $(DATA_PLANE_BUILD_DIR)
	go build -o $(DATA_PLANE_BUILD_DIR)/$(DATA_PLANE_BINARY_NAME) ./data-plane/cmd


run-control-plane:
	cd control-plane && mvn spring-boot:run

run-data-plane: build
	$(MAKE) -C $(DATA_PLANE_BUILD_DIR) all -j 

run: run-data-plane run-control-plane

docker:
	docker compose -f infra/docker-compose.yml down && \
	docker compose -f infra/docker-compose.yml up -d && \
	infra/vault/init-pki.sh && infra/vault/init-proxyclient-certs.sh

recreate-cluster:
	infra/k8s/recreate-cluster.sh

build-control-plane-image:
	docker build -t "sentinel-control-plane:latest" control-plane/
	kind load docker-image sentinel-control-plane:latest --name sentinel-zt
	helm upgrade --install control-plane infra/k8s/control-plane \
		-n sentinel-control-plane --create-namespace \
		--set rbac.create=true --set labels.app=control-plane
	kubectl rollout restart deployment -n sentinel-control-plane

build-data-plane-image:
	docker build -t "sentinel-data-plane:latest" data-plane/
	kind load docker-image sentinel-data-plane:latest --name sentinel-zt
	helm upgrade --install data-plane infra/k8s/data-plane \
  -n sentinel-data-plane --create-namespace \
  --set serviceAccount.create=true --set service.type=NodePort --set service.nodePort=30443
	kubectl rollout restart deployment -n sentinel-data-plane

build-images: build-control-plane-image build-data-plane-image

reload-pod-images: build-images

refresh-dashboard:
	infra/k8s/observability/refresh-dashboard.sh

reboot-vault:
	infra/k8s/vault/boot-vault.sh -a -p 8210

unseal-vault:
	infra/k8s/vault/boot-vault.sh -u
