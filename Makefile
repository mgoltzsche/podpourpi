BUILD_DIR=$(CURDIR)/build
BIN_DIR=$(BUILD_DIR)/bin
TOOLS_DIR=$(BUILD_DIR)/tools
OAPI_CODEGEN_VERSION = v1.9.0
OAPI_CODEGEN = $(TOOLS_DIR)/oapi-codegen
DEEPCOPY_GEN_VERSION=590cda81e5047108beb56c0532422f10ff4d8917
DEEPCOPY_GEN = $(TOOLS_DIR)/deepcopy-gen
SPECTRAL_OPENAPI_VALIDATOR_VERSION = 6.1.0
SPECTRAL_OPENAPI_VALIDATOR_NPM = $(TOOLS_DIR)/spectral-cli-$(SPECTRAL_OPENAPI_VALIDATOR_VERSION)
SPECTRAL_OPENAPI_VALIDATOR = $(SPECTRAL_OPENAPI_VALIDATOR_NPM)/node_modules/@stoplight/spectral-cli/dist/index.js
PRISM_MOCK_SERVER_VERSION = 4.5.0
PRISM_MOCK_SERVER_NPM = $(TOOLS_DIR)/prism-cli-$(PRISM_MOCK_SERVER_VERSION)
PRISM_MOCK_SERVER = $(PRISM_MOCK_SERVER_NPM)/node_modules/@stoplight/prism-cli/dist/index.js
KUBE_OPENAPI_GEN = $(TOOLS_DIR)/openapi-gen
KUBE_OPENAPI_GEN_VERSION = e816edb12b65975944ee23073f35617fd9d49215

CONTROLLER_GEN = $(TOOLS_DIR)/controller-gen
CONTROLLER_GEN_VERSION = v0.4.1

OPENAPI_FILE=./api/openapi.yaml

all: build

.PHONY: build
build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/podpourpi ./cmd/podpourpi

.PHONY: ui
ui: ui-build ui-type-check

ui-build: ui-generate
	cd ui && npm run build

ui-generate: ui/node_modules
	cd ui && npm run openapi:generate

ui-lint: ui/node_modules
	cd ui && npm run lint

ui-type-check: ui/node_modules
	cd ui && npm run vti

ui/node_modules:
	cd ui && npm install

clean:
	rm -rf "$(CURDIR)/build"

generate: $(CONTROLLER_GEN) $(OAPI_CODEGEN) $(DEEPCOPY_GEN) $(KUBE_OPENAPI_GEN)
	#PATH="$(TOOLS_DIR):$$PATH" go generate ./...
	#$(CONTROLLER_GEN) crd paths=k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1 output:dir=./crd
	#$(CONTROLLER_GEN) crd paths=./pkg/apis/... output:dir=./crd
	$(CONTROLLER_GEN) object paths=./pkg/apis/...
	$(KUBE_OPENAPI_GEN) --output-base=./pkg/generated --output-package=openapi -O zz_generated.openapi -h ./boilerplate/boilerplate.go.txt \
		--input-dirs=github.com/mgoltzsche/podpourpi/pkg/apis/app/v1alpha1,k8s.io/api/core/v1,k8s.io/kube-aggregator/pkg/apis/apiregistration/v1,k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/runtime,k8s.io/apimachinery/pkg/version,k8s.io/apimachinery/pkg/util/intstr

generate-openapi: generate build
	@echo Load OpenAPI spec from freshly built server binary
	@{ \
	set -eu; \
	$(BIN_DIR)/podpourpi serve & \
	PID=$$!; \
	sleep 1; \
	printf '# This file is generated using `make generate-openapi`.\n# DO NOT EDIT MANUALLY!\n\n' > openapi.yaml; \
	curl -fsS http://localhost:8080/openapi/v2 >> openapi.yaml; \
	kill -9 $$PID; \
	}

validate-openapi: ui/node_modules
	@echo Validating the OpenAPI spec at $(OPENAPI_FILE)
	cd ui && npm run openapi:validate

$(OAPI_CODEGEN): ## Installs oapi-codegen
	$(call go-get-tool,$(OAPI_CODEGEN),github.com/deepmap/oapi-codegen/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION))

$(DEEPCOPY_GEN):
	$(call go-get-tool,$(DEEPCOPY_GEN),k8s.io/gengo/examples/deepcopy-gen@$(DEEPCOPY_GEN_VERSION))

$(CONTROLLER_GEN):
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION))

$(KUBE_OPENAPI_GEN):
	$(call go-get-tool,$(KUBE_OPENAPI_GEN),k8s.io/kube-openapi/cmd/openapi-gen@$(KUBE_OPENAPI_GEN_VERSION))

# go-get-tool will 'go get' any package $2 and install it to $1.
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(TOOLS_DIR) go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
