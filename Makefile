IMAGE_TAG      := $(shell date +%s)
IMAGE_REGISTRY := docker.io/alesr
TARGET_ARCH    := linux/arm64
NAMESPACE      := worker-scaler
PROJECT_NAME   := worker-scaler-controller
DOCKER         := docker
KUBECTL        := kubectl

.PHONY: help
help: ## Show this help message
	@echo "------------------------------------------------------------------------"
	@echo "${PROJECT_NAME} (Target: Raspberry Pi ARM64 | Namespace: ${NAMESPACE})"
	@echo "------------------------------------------------------------------------"
	@awk 'BEGIN {FS = ":.*?## "}; $$0 ~ "^[[:alnum:]_/%-]+:.*?## " {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST) | sort

.PHONY: fmt
fmt: ## Format code
	go fmt ./...

.PHONY: vet
vet: ## Vet code
	go vet ./...

.PHONY: test
test: ## Run unit tests
	go test -short -race -count=1 -v ./...

.PHONY: lint
lint: vet ## Lint code
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
	else \
		echo "staticcheck not installed, skipping (go install honnef.co/go/tools/cmd/staticcheck@latest)"; \
	fi

.PHONY: create-namespace
create-namespace: ## Create the namespace
	$(KUBECTL) apply -f manifests/namespace.yaml

.PHONY: build-producer
build-producer: ## Build the producer image
	$(DOCKER) build --platform $(TARGET_ARCH) -f build/Dockerfile --target producer -t $(IMAGE_REGISTRY)/redis-producer:$(IMAGE_TAG) .

.PHONY: build-worker
build-worker: ## Build the worker image
	$(DOCKER) build --platform $(TARGET_ARCH) -f build/Dockerfile --target worker -t $(IMAGE_REGISTRY)/redis-worker:$(IMAGE_TAG) .

.PHONY: build-controller
build-controller: ## Build the scaler controller image
	$(DOCKER) build --platform $(TARGET_ARCH) -f build/Dockerfile --target controller -t $(IMAGE_REGISTRY)/scaler-controller:$(IMAGE_TAG) .

.PHONY: build-all
build-all: build-producer build-worker build-controller #£ Build all images

.PHONY: push-all
push-all: build-all ## Build and push all images
	$(DOCKER) push $(IMAGE_REGISTRY)/redis-producer:$(IMAGE_TAG)
	$(DOCKER) push $(IMAGE_REGISTRY)/redis-worker:$(IMAGE_TAG)
	$(DOCKER) push $(IMAGE_REGISTRY)/scaler-controller:$(IMAGE_TAG)

.PHONY: deploy-redis
deploy-redis: create-namespace ## Deploy Redis to the Pi cluster
	$(KUBECTL) apply -n $(NAMESPACE) -f manifests/redis-service.yaml
	$(KUBECTL) apply -n $(NAMESPACE) -f manifests/redis-deployment.yaml

.PHONY: deploy-worker
deploy-worker: create-namespace ## Deploy the Worker pool to the Pi cluster
	sed -i '' 's|image: .*/redis-worker:.*|image: $(IMAGE_REGISTRY)/redis-worker:$(IMAGE_TAG)|g' manifests/worker-deployment.yaml
	$(KUBECTL) apply -n $(NAMESPACE) -f manifests/worker-deployment.yaml
	$(KUBECTL) rollout restart deployment/redis-worker -n $(NAMESPACE)

.PHONY: deploy-controller
deploy-controller: create-namespace ## Deploy the Custom Controller + RBAC to the Pi cluster
	$(KUBECTL) apply -n $(NAMESPACE) -f manifests/controller-rbac.yaml
	sed -i '' 's|image: .*/scaler-controller:.*|image: $(IMAGE_REGISTRY)/scaler-controller:$(IMAGE_TAG)|g' manifests/controller-deployment.yaml
	$(KUBECTL) apply -n $(NAMESPACE) -f manifests/controller-deployment.yaml
	$(KUBECTL) rollout restart deployment/scaler-controller -n $(NAMESPACE)

.PHONY: deploy-all
deploy-all: push-all deploy-redis deploy-worker deploy-controller ## Deploy all images

.PHONY: tear-down
tear-down: ## Completely wipe the environment from the Pi
	$(KUBECTL) delete namespace $(NAMESPACE) --ignore-not-found=true
