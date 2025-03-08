ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

GO ?= go
GIT ?= git
KUBECTL ?= kubectl
DOCKER ?= docker

.PHONY: manifests
manifests: controller-gen
	@$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: config
config: manifests

.PHONY: generate
generate: controller-gen
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: api
api: generate

.PHONY: fmt vet test
fmt vet test:
	@$(GO) $@ ./...

.PHONY: download vendor verify
download vendor verify:
	@$(GO) mod $@

.PHONY: lint
lint: golangci-lint fmt
	@$(GOLANGCI_LINT) config verify
	@$(GOLANGCI_LINT) run --fix

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize
	@$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize
	@$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	@mkdir -p $(LOCALBIN)

APPA ?= $(LOCALBIN)/appa
KUBECTL_UPLOAD_APP ?= $(LOCALBIN)/kubectl-upload_app
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
SWAG ?= $(LOCALBIN)/swag

KUSTOMIZE_VERSION ?= v5.5.0
CONTROLLER_TOOLS_VERSION ?= v0.17.1
GOLANGCI_LINT_VERSION ?= v1.63.4
SWAG_VERSION ?= v1.16.4

.PHONY: appa
appa: $(APPA)
$(APPA): $(LOCALBIN)
	@$(GO) build -o $@ ./cmd/appa

.PHONY: kubectl-upload_app
kubectl-upload_app: $(KUBECTL_UPLOAD_APP)
$(KUBECTL_UPLOAD_APP): $(LOCALBIN)
	@$(GO) build -o $@ ./cmd/kubectl-upload_app

.PHONY: kustomize
kustomize: $(KUSTOMIZE)
$(KUSTOMIZE): $(LOCALBIN)
	@$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): $(LOCALBIN)
	@$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT)
$(GOLANGCI_LINT): $(LOCALBIN)
	@$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: swag
swag: $(SWAG)
$(SWAG): $(LOCALBIN)
	@$(call go-install-tool,$(SWAG),github.com/swaggo/swag/cmd/swag,$(SWAG_VERSION))

define go-install-tool
@[ -f "$(1)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) $(GO) install $${package} ;\
} ;
endef

.PHONY: testdata/momo/node_modules
testdata/momo/node_modules:
	@cd testdata/momo && yarn

.PHONY: testdata/momo.apk
testdata/momo.apk: testdata/momo/node_modules
	@cd testdata/momo && yarn android
	@cp testdata/momo/android/app/build/outputs/apk/debug/app-debug.apk $@
	@jarsigner -sigalg SHA1withRSA -digestalg SHA1 -storepass android -keypass android -keystore testdata/momo/android/app/debug.keystore $@ androiddebugkey

.PHONY: internal/api
internal/api: swag
	@$(SWAG) fmt --dir $@
	@$(SWAG) init --dir $@ --output $@ --outputTypes json --parseInternal
	@echo >> $@/swagger.json

.PHONY: test-upload
test-upload: appa
	@$(DOCKER) compose up -d
	@$(KUBECTL) apply -f config/samples/momo_v1alpha1_bucket.yaml
	@$(APPA) upload app default testdata/momo.apk
