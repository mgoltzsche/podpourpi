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

OPENAPI_FILE=./api/openapi.yaml

all: build

.PHONY: build
build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/podpourpi ./cmd/podpourpi

clean:
	rm -rf "$(CURDIR)/build"

generate: $(OAPI_CODEGEN) $(DEEPCOPY_GEN)
	PATH="$(TOOLS_DIR):$$PATH" go generate ./...

# TODO: make openapi validation and code generation work
validate-openapi: $(SPECTRAL_OPENAPI_VALIDATOR)
	@echo Validating the OpenAPI spec at $(OPENAPI_FILE)
	@$(SPECTRAL_OPENAPI_VALIDATOR) lint -r spectral.openapi.yaml $(OPENAPI_FILE)

mock-server: $(PRISM_MOCK_SERVER)
	$(PRISM_MOCK_SERVER) mock $(OPENAPI_FILE)

$(OAPI_CODEGEN): ## Installs oapi-codegen
	$(call go-get-tool,$(OAPI_CODEGEN),github.com/deepmap/oapi-codegen/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION))

$(CONTROLLER_GEN):
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

$(DEEPCOPY_GEN):
	$(call go-get-tool,$(DEEPCOPY_GEN),k8s.io/gengo/examples/deepcopy-gen@$(DEEPCOPY_GEN_VERSION))

$(SPECTRAL_OPENAPI_VALIDATOR):
	$(call npm-get-tool,$(SPECTRAL_OPENAPI_VALIDATOR),$(SPECTRAL_OPENAPI_VALIDATOR_NPM),@stoplight/spectral-cli@$(SPECTRAL_OPENAPI_VALIDATOR_VERSION))

$(PRISM_MOCK_SERVER):
	$(call npm-get-tool,$(PRISM_MOCK_SERVER),$(PRISM_MOCK_SERVER_NPM),@stoplight/prism-cli@$(PRISM_MOCK_SERVER_VERSION))

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

# npm-get-tool downloads a tool $3 using 'npm install' within directory $2 if $1 does not exist already
define npm-get-tool
@[ -f "$(1)" ] || { \
echo "Downloading $(3) from npm" ;\
set -e ;\
mkdir -p "$(2)" ;\
cd "$(2)" ;\
npm init -y >/dev/null;\
npm install "$(3)" ;\
}
endef
