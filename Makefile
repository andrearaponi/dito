# Verification tools and CI targets (S-02): tool versions live in tools.mk.
include tools.mk

# Let an older local Go switch to the toolchain required by go.mod.
export GOTOOLCHAIN ?= auto

# GO_CMD: The command to run Go.
# GO_BUILD: The command to build the Go project.
# GO_TEST: The command to run Go tests.
# GO_VET: The command to run Go vet.
# GO_FMT: The command to format Go code.
# BINARY_NAME: The name of the binary to be created.
# PKG: The package to be used for Go commands.
# API_DIR: The directory containing the API source code.
# CONFIG_FILE: The path to the configuration file.
GO_CMD=go
GO_BUILD=$(GO_CMD) build
GO_TEST=$(GO_CMD) test
GO_VET=$(GO_CMD) vet
GO_FMT=$(GO_CMD) fmt
BINARY_NAME=dito
PKG=./...
API_DIR=cmd
CONFIG_FILE=cmd/config.yaml
PLUGINS_DIR=plugins
PLUGIN_SIGNER_DIR=cmd/plugin-signer
PLUGIN_SIGNER_BINARY=plugin-signer

# Key files (in bin directory)
PUBLIC_KEY_FILE=bin/ed25519_public.key
PRIVATE_KEY_FILE=bin/ed25519_private.key

# SONAR_HOST_URL: The URL of the SonarQube server.
# SONAR_PROJECT_KEY: The unique key for the SonarQube project.
SONAR_HOST_URL=http://localhost:9000
SONAR_PROJECT_KEY=dito

# .PHONY: Declares phony targets that are not actual files.
.PHONY: build setup setup-prod build-plugin-signer generate-keys generate-prod-keys sonar test vet fmt clean run build-plugins clean-plugins sign-plugins sign-plugins-prod update-config update-prod-config update-k8s-config quick-start help debug-config deploy-ocp deploy-ocp-dev clean-ocp status-ocp logs-ocp

# setup: Complete setup for development - builds everything and generates keys if needed
setup: build-plugin-signer generate-keys build build-plugins sign-plugins update-config
	@echo "✅ Development setup complete! You can now run: make run"

# setup-prod: Complete setup for production - uses persistent keys and creates production config
setup-prod: build-plugin-signer generate-prod-keys build build-plugins sign-plugins-prod update-prod-config
	@echo "✅ Production setup complete!"
	@echo "📦 Production files ready:"
	@echo "  - bin/$(BINARY_NAME) (application binary)"
	@echo "  - bin/config-prod.yaml (production config)"
	@echo "  - bin/ed25519_public_prod.key (production public key)"
	@echo "  - bin/ed25519_private_prod.key (production private key)"
	@echo "🚀 Ready for containerization with persistent keys!"

# build: Compiles the Go project and copies the configuration file to the bin directory.
build:
	@echo "🔨 Building Dito..."
	@mkdir -p bin
	@$(GO_BUILD) -o bin/$(BINARY_NAME) $(API_DIR)/*.go && cp $(CONFIG_FILE) bin/
	@echo "✅ Dito built successfully"
	@echo "📄 Config copied: $(CONFIG_FILE) → bin/config.yaml"

# build-plugin-signer: Builds the plugin signer tool
build-plugin-signer:
	@echo "🔨 Building plugin-signer..."
	@mkdir -p bin
	@cd $(PLUGIN_SIGNER_DIR) && $(GO_BUILD) -o ../../bin/$(PLUGIN_SIGNER_BINARY) .
	@echo "✅ Plugin-signer built successfully"

# generate-keys: Generates Ed25519 key pair if they don't exist
generate-keys: build-plugin-signer
	@mkdir -p bin
	@if [ ! -f $(PUBLIC_KEY_FILE) ] || [ ! -f $(PRIVATE_KEY_FILE) ]; then \
		echo "🔑 Generating Ed25519 key pair..."; \
		cd bin && ../bin/$(PLUGIN_SIGNER_BINARY) generate-keys; \
		echo "✅ Keys generated successfully in bin/ directory"; \
	else \
		echo "🔑 Keys already exist in bin/ directory, skipping generation"; \
	fi

# generate-prod-keys: Generates persistent Ed25519 key pair for production (only if they don't exist)
generate-prod-keys: build-plugin-signer
	@mkdir -p bin
	@if [ ! -f bin/ed25519_public_prod.key ] || [ ! -f bin/ed25519_private_prod.key ]; then \
		echo "🔑 Generating persistent Ed25519 key pair for production..."; \
		cd bin && ../bin/$(PLUGIN_SIGNER_BINARY) generate-keys; \
		mv ed25519_public.key ed25519_public_prod.key; \
		mv ed25519_private.key ed25519_private_prod.key; \
		echo "Keys generated successfully: ed25519_public_prod.key, ed25519_private_prod.key"; \
		echo "✅ Production keys generated successfully in bin/ directory"; \
	else \
		echo "🔑 Production keys already exist in bin/ directory, keeping existing keys for consistency"; \
	fi

# Build all plugins dynamically
build-plugins:
	@echo "🔨 Building plugins..."
	@find plugins -mindepth 1 -maxdepth 1 -type d -exec sh -c 'echo "Building plugin: {}" && cd {} && go build -buildmode=plugin -o $$(basename {}).so' \;
	@echo "✅ Plugins built successfully"

# sign-plugins: Signs all plugins automatically
sign-plugins: generate-keys
	@echo "🔏 Signing plugins..."
	@find plugins -name "*.so" -type f | while read plugin; do \
		if [ ! -f "$$plugin.sig" ]; then \
			echo "Signing $$plugin..."; \
			cp $(PRIVATE_KEY_FILE) ed25519_private.key; \
			./bin/$(PLUGIN_SIGNER_BINARY) sign "$$plugin"; \
			rm ed25519_private.key; \
		else \
			echo "$$plugin already signed, skipping"; \
		fi \
	done
	@echo "✅ Plugins signed successfully"

# sign-plugins-prod: Signs all plugins automatically with production keys
sign-plugins-prod: generate-prod-keys
	@echo "🔏 Signing plugins with production keys..."
	@find plugins -name "*.so" -type f | while read plugin; do \
		echo "Signing $$plugin with production key..."; \
		cp bin/ed25519_private_prod.key ed25519_private.key; \
		./bin/$(PLUGIN_SIGNER_BINARY) sign "$$plugin"; \
		rm ed25519_private.key; \
	done
	@echo "✅ Plugins signed successfully with production keys"

# update-config: Updates bin/config.yaml with the correct public key hash and paths
update-config: generate-keys build
	@echo "🔧 Updating bin/config.yaml with public key hash and paths..."
	@if [ ! -f $(PUBLIC_KEY_FILE) ]; then \
		echo "❌ Public key file not found: $(PUBLIC_KEY_FILE)"; \
		exit 1; \
	fi
	@if [ ! -f bin/config.yaml ]; then \
		echo "❌ bin/config.yaml not found. Run 'make build' first."; \
		exit 1; \
	fi
	@if command -v shasum >/dev/null 2>&1; then \
		HASH=$$(shasum -a 256 $(PUBLIC_KEY_FILE) | awk '{print $$1}'); \
	elif command -v sha256sum >/dev/null 2>&1; then \
		HASH=$$(sha256sum $(PUBLIC_KEY_FILE) | awk '{print $$1}'); \
	else \
		echo "❌ Neither shasum nor sha256sum found. Please install one of them."; \
		exit 1; \
	fi; \
	echo "📝 Current public key hash: $$HASH"; \
	echo "🔧 Updating bin/config.yaml..."; \
	echo "📋 Before update:"; \
	grep -A1 -B1 "public_key" bin/config.yaml || echo "  (public_key lines not found)"; \
	sed -i.bak 's|directory: "[^"]*"|directory: "../plugins"|' bin/config.yaml; \
	sed -i.bak 's|public_key_path: "[^"]*"|public_key_path: "./ed25519_public.key"|' bin/config.yaml; \
	sed -i.bak 's|public_key_hash: "[^"]*"[^"]*|public_key_hash: "'$$HASH'"|' bin/config.yaml; \
	echo "📋 After update:"; \
	grep -A3 -B1 "plugins:" bin/config.yaml || echo "  (plugins section not found)"; \
	echo "✅ bin/config.yaml updated successfully"

# update-prod-config: Updates bin/config-prod.yaml with the correct public key hash and paths for production
update-prod-config: generate-prod-keys build
	@echo "🔧 Creating and updating bin/config-prod.yaml with production public key hash and paths..."
	@if [ ! -f bin/ed25519_public_prod.key ]; then \
		echo "❌ Production public key file not found: bin/ed25519_public_prod.key"; \
		exit 1; \
	fi
	@# Copy the base config to production config
	@cp bin/config.yaml bin/config-prod.yaml
	@if command -v shasum >/dev/null 2>&1; then \
		HASH=$$(shasum -a 256 bin/ed25519_public_prod.key | awk '{print $$1}'); \
	elif command -v sha256sum >/dev/null 2>&1; then \
		HASH=$$(sha256sum bin/ed25519_public_prod.key | awk '{print $$1}'); \
	else \
		echo "❌ Neither shasum nor sha256sum found. Please install one of them."; \
		exit 1; \
	fi; \
	echo "📝 Production public key hash: $$HASH"; \
	echo "🔧 Updating bin/config-prod.yaml..."; \
	echo "📋 Before update:"; \
	grep -A1 -B1 "public_key" bin/config-prod.yaml || echo "  (public_key lines not found)"; \
	sed -i.bak 's|directory: "[^"]*"|directory: "./plugins"|' bin/config-prod.yaml; \
	sed -i.bak 's|public_key_path: "[^"]*"|public_key_path: "./ed25519_public_prod.key"|' bin/config-prod.yaml; \
	sed -i.bak 's|public_key_hash: "[^"]*"[^"]*|public_key_hash: "'$$HASH'"|' bin/config-prod.yaml; \
	echo "📋 After update:"; \
	grep -A3 -B1 "plugins:" bin/config-prod.yaml || echo "  (plugins section not found)"; \
	echo "✅ bin/config-prod.yaml updated successfully"

# update-k8s-config: Creates Kubernetes-specific config with correct paths
update-k8s-config: generate-prod-keys
	@echo "🔧 Creating Kubernetes configuration from template..."
	@if [ ! -f bin/ed25519_public_prod.key ]; then \
		echo "❌ Production public key file not found: bin/ed25519_public_prod.key"; \
		exit 1; \
	fi
	@if command -v shasum >/dev/null 2>&1; then \
		HASH=$$(shasum -a 256 bin/ed25519_public_prod.key | awk '{print $$1}'); \
	elif command -v sha256sum >/dev/null 2>&1; then \
		HASH=$$(sha256sum bin/ed25519_public_prod.key | awk '{print $$1}'); \
	else \
		echo "❌ Neither shasum nor sha256sum found. Please install one of them."; \
		exit 1; \
	fi; \
	echo "📝 Production public key hash: $$HASH"; \
	echo "🔧 Creating bin/config-prod-k8s.yaml from template..."; \
	sed "s/PLACEHOLDER_HASH_TO_BE_REPLACED/$$HASH/" configs/templates/application.yaml > configs/config-prod-k8s.yaml; \
	echo "✅ Kubernetes config created: configs/config-prod-k8s.yaml"

# debug-config: Debug configuration issues
debug-config:
	@echo "🔍 Debugging configuration..."
	@echo "📁 Files in bin/:"
	@ls -la bin/ || echo "bin/ directory doesn't exist"
	@echo ""
	@echo "🔑 Public key file:"
	@if [ -f $(PUBLIC_KEY_FILE) ]; then \
		echo "  ✅ $(PUBLIC_KEY_FILE) exists"; \
		HASH=$$(shasum -a 256 $(PUBLIC_KEY_FILE) | awk '{print $$1}'); \
		echo "  📝 Hash: $$HASH"; \
	else \
		echo "  ❌ $(PUBLIC_KEY_FILE) not found"; \
	fi
	@echo ""
	@echo "📄 Configuration file:"
	@if [ -f bin/config.yaml ]; then \
		echo "  ✅ bin/config.yaml exists"; \
		echo "  📋 Plugin configuration in bin/config.yaml:"; \
		grep -A5 "plugins:" bin/config.yaml || echo "  (plugins section not found)"; \
	else \
		echo "  ❌ bin/config.yaml not found"; \
	fi

# Clean all compiled plugin binaries and signatures
clean-plugins:
	@echo "🧹 Cleaning plugins..."
	@find plugins -name "*.so" -type f -delete
	@find plugins -name "*.so.sig" -type f -delete

# vet: Runs go vet on the main module and on every plugin module.
vet:
	@$(GO_VET) $(PKG)
	@for dir in plugins/*/; do (cd "$$dir" && $(GO_VET) ./...) || exit 1; done
	@echo "vet: ok"

# fmt: Formats the Go code.
fmt:
	$(GO_FMT) $(PKG)

# clean: Removes the binary, configuration file, and compiled plugins.
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -f bin/$(BINARY_NAME) bin/$(PLUGIN_SIGNER_BINARY) bin/config.yaml $(PUBLIC_KEY_FILE) $(PRIVATE_KEY_FILE) && $(MAKE) clean-plugins

# run: Runs the compiled binary.
run:
	@if [ ! -f bin/$(BINARY_NAME) ]; then \
		echo "❌ Dito binary not found. Run 'make setup' first."; \
		exit 1; \
	fi
	@if [ ! -f bin/config.yaml ]; then \
		echo "❌ bin/config.yaml not found. Run 'make setup' first."; \
		exit 1; \
	fi
	@echo "🚀 Starting Dito from bin/ directory..."
	@cd bin && ./$(BINARY_NAME)

# quick-start: One command to get everything running
quick-start: clean setup
	@echo "🚀 Starting Dito..."
	@$(MAKE) run

# fix-config: Quick command to fix configuration after setup
fix-config: update-config
	@echo "✅ Configuration fixed!"

# test: Runs the Go tests.
test:
	$(GO_TEST) $(PKG)

# sonar: Analyzes the project with SonarQube.
sonar:
	sonar-scanner  \
 	 	-Dsonar.projectKey=$(SONAR_PROJECT_KEY) \
 	 	-Dsonar.sources=. \
  		-Dsonar.host.url=$(SONAR_HOST_URL) \
  		-Dsonar.token=$(SONAR_DITO_TOKEN)

# help: Shows available commands
help:
	@echo ""
	@echo "🔧 Dito Build Commands"
	@echo "======================"
	@echo ""
	@echo "🚀 Quick Commands:"
	@echo "  make quick-start     - Clean setup everything and start server"
	@echo "  make setup           - Complete development setup (build, keys, plugins)"
	@echo "  make setup-prod      - Complete production setup (persistent keys, prod config)"
	@echo "  make fix-config      - Fix bin/config.yaml with correct paths/hashes"
	@echo ""
	@echo "🔨 Build Commands:"
	@echo "  make build           - Build Dito binary only"
	@echo "  make build-plugins   - Build all plugins"
	@echo "  make build-plugin-signer - Build plugin signer tool"
	@echo ""
	@echo "🔑 Security Commands:"
	@echo "  make generate-keys   - Generate Ed25519 key pair for development"
	@echo "  make generate-prod-keys - Generate persistent Ed25519 key pair for production"
	@echo "  make sign-plugins    - Sign all plugins with development keys"
	@echo "  make sign-plugins-prod - Sign all plugins with production keys"
	@echo "  make update-config   - Update bin/config.yaml with development key paths/hashes"
	@echo "  make update-prod-config - Update bin/config-prod.yaml with production key paths/hashes"
	@echo "  make update-k8s-config - Create bin/config-prod-k8s.yaml for Kubernetes deployment"
	@echo ""
	@echo "🎮 Runtime Commands:"
	@echo "  make run             - Run Dito server"
	@echo ""
	@echo "🔍 Debug Commands:"
	@echo "  make debug-config    - Debug configuration issues"
	@echo ""
	@echo "🧹 Cleanup Commands:"
	@echo "  make clean           - Clean all build artifacts"
	@echo "  make clean-plugins   - Clean plugin binaries only"
	@echo ""
	@echo "🧪 Development Commands:"
	@echo "  make test            - Run tests"
	@echo "  make vet             - Run go vet"
	@echo "  make fmt             - Format code"
	@echo "  make sonar           - Run SonarQube analysis"
	@echo ""
	@echo "🚀 OpenShift Deployment:"
	@echo "  make deploy-ocp      - Complete OpenShift production deployment"
	@echo "  make deploy-ocp-dev  - Quick development deployment"
	@echo "  make status-ocp      - Check OpenShift deployment status"
	@echo "  make logs-ocp        - View OpenShift deployment logs"
	@echo "  make clean-ocp       - Clean up OpenShift resources"
	@echo ""
	@echo "❓ Help:"
	@echo "  make help            - Show this help"
	@echo ""

# deploy-ocp: Complete OpenShift deployment with all components
deploy-ocp: setup-prod update-k8s-config
	@echo "🚀 Starting complete OpenShift deployment..."
	@if ! command -v oc >/dev/null 2>&1; then \
		echo "❌ OpenShift CLI (oc) not found. Please install it."; \
		exit 1; \
	fi
	@if ! oc whoami >/dev/null 2>&1; then \
		echo "❌ Not logged into OpenShift. Please run: oc login <cluster-url>"; \
		exit 1; \
	fi
	@echo "📦 Building and pushing container image..."
	@./docker-build.sh
	@echo "🔧 Deploying with automated script..."
	@./scripts/deploy-ocp.sh
	@echo "✅ Complete OpenShift deployment finished!"

# deploy-ocp-dev: Quick deployment for development/testing
deploy-ocp-dev: setup
	@echo "🔧 Starting development OpenShift deployment..."
	@if ! command -v oc >/dev/null 2>&1; then \
		echo "❌ OpenShift CLI (oc) not found. Please install it."; \
		exit 1; \
	fi
	@if ! oc whoami >/dev/null 2>&1; then \
		echo "❌ Not logged into OpenShift. Please run: oc login <cluster-url>"; \
		exit 1; \
	fi
	@echo "📦 Building and pushing container image..."
	@VERSION=dev ./docker-build.sh
	@echo "🔧 Creating development resources..."
	@NAMESPACE=$${NAMESPACE:-dito-dev} ./scripts/deploy-ocp.sh -v dev
	@echo "✅ Development deployment completed!"

# clean-ocp: Clean up OpenShift resources
clean-ocp:
	@echo "🧹 Cleaning up OpenShift resources..."
	@if ! command -v oc >/dev/null 2>&1; then \
		echo "❌ OpenShift CLI (oc) not found. Please install it."; \
		exit 1; \
	fi
	@NAMESPACE=$${NAMESPACE:-dito}; \
	echo "🗑️  Deleting resources from namespace: $$NAMESPACE"; \
	oc delete all,configmap,secret,networkpolicy,hpa,pdb -l app=dito -n $$NAMESPACE 2>/dev/null || echo "No resources found"; \
	echo "✅ OpenShift cleanup completed"

# status-ocp: Check OpenShift deployment status
status-ocp:
	@echo "📊 Checking OpenShift deployment status..."
	@if ! command -v oc >/dev/null 2>&1; then \
		echo "❌ OpenShift CLI (oc) not found. Please install it."; \
		exit 1; \
	fi
	@NAMESPACE=$${NAMESPACE:-dito}; \
	echo "📍 Namespace: $$NAMESPACE"; \
	echo ""; \
	echo "🚀 Deployments:"; \
	oc get deployment -l app=dito -n $$NAMESPACE 2>/dev/null || echo "No deployments found"; \
	echo ""; \
	echo "📦 Pods:"; \
	oc get pods -l app=dito -n $$NAMESPACE 2>/dev/null || echo "No pods found"; \
	echo ""; \
	echo "🔗 Services:"; \
	oc get svc -l app=dito -n $$NAMESPACE 2>/dev/null || echo "No services found"; \
	echo ""; \
	echo "🌐 Routes:"; \
	oc get route -l app=dito -n $$NAMESPACE 2>/dev/null || echo "No routes found"; \
	echo ""; \
	echo "🔒 Secrets:"; \
	oc get secret -l app=dito -n $$NAMESPACE 2>/dev/null || echo "No secrets found"; \
	echo ""; \
	echo "📋 ConfigMaps:"; \
	oc get configmap -l app=dito -n $$NAMESPACE 2>/dev/null || echo "No configmaps found"

# logs-ocp: View OpenShift deployment logs
logs-ocp:
	@echo "📋 Viewing OpenShift deployment logs..."
	@if ! command -v oc >/dev/null 2>&1; then \
		echo "❌ OpenShift CLI (oc) not found. Please install it."; \
		exit 1; \
	fi
	@NAMESPACE=$${NAMESPACE:-dito}; \
	echo "📍 Namespace: $$NAMESPACE"; \
	oc logs -l app=dito -n $$NAMESPACE --tail=100 -f

# ---------------------------------------------------------------------------
# Continuous integration (S-02). Each CI job runs exactly one of these targets
# and every target prints "<target>: ok" only when all its checks pass.
# ---------------------------------------------------------------------------

.PHONY: ci tools-check ci-selftest build-check modules test-race test-hermetic image lint lint-all vuln smoke-plugins coverage workflows spec-validate e2e

# tools-check: Verifies tool versions (tools.mk) and the Go that builds them (go.mod).
tools-check:
	@scripts/ci/tools-check.sh

# ci-selftest: Mutation harness proving each gate fails on its defect (SELFTEST_CASES selects cases).
ci-selftest:
	@SELFTEST_CASES="$(SELFTEST_CASES)" scripts/ci/selftest.sh

# build-check: Builds the proxy, the plugin-signer and every plugin; outputs go to a temporary directory.
build-check:
	@go build ./...
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
	for dir in plugins/*/; do \
		name=$$(basename "$$dir"); \
		(cd "$$dir" && go build -buildmode=plugin -o "$$tmp/$$name.so" .) || exit 1; \
	done
	@echo "build-check: ok"

# modules: Tidy go.mod/go.sum, one Go version everywhere, same versions for modules shared by host and plugins.
modules:
	@scripts/ci/modules-check.sh

# test-race: Runs every test with the race detector, in random order (-shuffle), without caching.
test-race:
	@go test -race -shuffle=on -count=1 $$(go list ./... | grep -v '/e2e$$')
	@go test -race -shuffle=on -count=1 -v ./e2e/
	@echo "test-race: ok"

# test-hermetic: Runs the tests with outbound traffic restricted to loopback, plus a negative control.
# Test binaries are compiled first, outside the isolation, so the isolated run downloads nothing.
test-hermetic:
	@go test -count=1 -run '^$$' ./... > /dev/null
	@cd plugins/hello-plugin && go mod download
	@pkgs=$$(go list ./... | grep -v '/e2e$$') && scripts/ci/hermetic.sh go test -count=1 $$pkgs
	@scripts/ci/hermetic.sh go test -count=1 -v ./e2e/
	@scripts/ci/hermetic.sh sh -c 'curl -sS -m 3 -o /dev/null http://192.0.2.1/; rc=$$?; \
		if [ "$$rc" -ne 7 ]; then echo "test-hermetic: negative control failed: curl exit $$rc, expected 7 (connection refused by the isolation)" >&2; exit 1; fi'
	@echo "test-hermetic: negative control ok (external connection refused)"
	@echo "test-hermetic: ok"

CONTAINER_ENGINE ?= $(shell if docker info >/dev/null 2>&1; then echo docker; elif podman info >/dev/null 2>&1; then echo podman; fi)
IMAGE_TAG ?= dito:ci-check

# image: Builds the linux/amd64 container image without pushing it (docker or podman).
image:
	@if [ -z "$(CONTAINER_ENGINE)" ]; then echo "image: no container engine available (docker or podman)" >&2; exit 1; fi
	@$(CONTAINER_ENGINE) build --platform linux/amd64 -t $(IMAGE_TAG) -f Dockerfile .
	@echo "image: ok"

LINT_BASE ?= main
LINT_MODE ?= merge-base

# lint: Reports only the golangci-lint issues introduced after LINT_BASE (LINT_MODE: merge-base or rev).
lint:
	@if [ "$(LINT_MODE)" = rev ]; then new="--new-from-rev=$(LINT_BASE)"; else new="--new-from-merge-base=$(LINT_BASE)"; fi; \
	$(GOLANGCI_LINT) run $$new ./... && \
	for dir in plugins/*/; do (cd "$$dir" && $(GOLANGCI_LINT) run $$new ./...) || exit 1; done
	@echo "lint: ok"

# lint-all: Reports every golangci-lint issue, including the pre-existing debt.
lint-all:
	@$(GOLANGCI_LINT) run ./...
	@for dir in plugins/*/; do (cd "$$dir" && $(GOLANGCI_LINT) run ./...) || exit 1; done

VULN_DIRS ?= . $(wildcard plugins/*)
VULN_RUN_GOTOOLCHAIN ?=

# vuln: Runs govulncheck on the main module and on every plugin (VULN_DIRS).
# VULN_RUN_GOTOOLCHAIN scans with another Go standard library; the scanner is still built with go.mod's Go.
vuln:
	@for dir in $(VULN_DIRS); do \
		echo "vuln: scanning $$dir"; \
		(cd "$$dir" && $(GO_RUN_TOOL) $(if $(VULN_RUN_GOTOOLCHAIN),-exec "env GOTOOLCHAIN=$(VULN_RUN_GOTOOLCHAIN)") $(GOVULNCHECK_PKG)@$(GOVULNCHECK_VERSION) ./...) || exit 1; \
	done
	@echo "vuln: ok"

SMOKE_PORT ?= 18181
SMOKE_EXPECT_HEADER ?= X-Hello-Plugin
SMOKE_TAMPER ?=

# smoke-plugins: Builds, signs (throwaway keys) and loads the example plugin, then proxies a request through it.
smoke-plugins:
	@SMOKE_PORT="$(SMOKE_PORT)" SMOKE_EXPECT_HEADER="$(SMOKE_EXPECT_HEADER)" SMOKE_TAMPER="$(SMOKE_TAMPER)" scripts/ci/plugin-smoke.sh

COVERAGE_BASE ?= main
# COVERAGE_E2E=no skips the e2e scenarios after the gate: make ci-selftest uses it to test the gate alone.
COVERAGE_E2E ?= yes

# coverage: Per-package coverage table, floors (scripts/ci/coverage-floors.txt), floor ratchet against COVERAGE_BASE.
coverage:
	@COVERAGE_BASE="$(COVERAGE_BASE)" scripts/ci/coverage-gate.sh
	@if [ "$(COVERAGE_E2E)" != no ]; then go test -count=1 -v ./e2e/; fi

# workflows: Project policies on the GitHub workflows, then actionlint (without shellcheck/pyflakes: same result everywhere).
workflows:
	@scripts/ci/workflows-check.sh
	@$(ACTIONLINT) -shellcheck= -pyflakes=
	@echo "workflows: ok"

# spec-validate: Validates every Walden spec with the walden version pinned in tools.mk.
spec-validate:
	@for dir in .walden/specs/*/; do \
		feature=$$(basename "$$dir"); \
		if ! $(WALDEN) validate "$$feature" --all >/dev/null 2>&1; then \
			$(WALDEN) validate "$$feature" --all >&2; echo "spec-validate: $$feature is not valid" >&2; exit 1; \
		fi; \
		echo "spec-validate: $$feature valid"; \
	done
	@echo "spec-validate: ok"

# ci: Every CI check that needs no container engine, in pipeline order; stops at the first failure.
ci:
	@$(MAKE) --no-print-directory tools-check
	@$(MAKE) --no-print-directory build-check
	@$(MAKE) --no-print-directory vet
	@$(MAKE) --no-print-directory modules
	@$(MAKE) --no-print-directory test-race
	@$(MAKE) --no-print-directory test-hermetic
	@$(MAKE) --no-print-directory lint
	@$(MAKE) --no-print-directory vuln
	@$(MAKE) --no-print-directory coverage
	@$(MAKE) --no-print-directory smoke-plugins
	@$(MAKE) --no-print-directory workflows
	@echo "ci: ok"

# e2e: End-to-end scenarios of the proxy (e2e/), with the race detector and in random order.
# SCENARIO=<regex> selects scenarios or areas, for example SCENARIO=^TestLimits_ or SCENARIO=_R05_.
e2e:
	@go test -count=1 -race -shuffle=on -v $(if $(SCENARIO),-run '$(SCENARIO)') ./e2e/
