# Verification tools used by CI and by the local make targets (S-02).
#
# This file is the single place where their versions are pinned (R6.AC2).
# Renovate updates the versions through the custom manager in renovate.json,
# which reads the "# renovate:" comment above each version.
#
# Tools run with `go run module@version`, so they never enter the dependency
# graph of go.mod (shared with the plugins). They are always built with the Go
# version declared in go.mod (C4): with GOTOOLCHAIN=auto an older local Go
# would otherwise build them with the minimum version each tool requires, and
# golangci-lint refuses to analyze code newer than the Go that built it.

# renovate: datasource=go depName=github.com/golangci/golangci-lint/v2
GOLANGCI_LINT_VERSION := v2.14.0
# renovate: datasource=go depName=golang.org/x/vuln
GOVULNCHECK_VERSION := v1.8.0
# renovate: datasource=go depName=github.com/rhysd/actionlint
ACTIONLINT_VERSION := v1.7.12
# renovate: datasource=go depName=github.com/andrearaponi/walden
WALDEN_VERSION := v0.11.0

GOLANGCI_LINT_PKG := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOVULNCHECK_PKG := golang.org/x/vuln/cmd/govulncheck
ACTIONLINT_PKG := github.com/rhysd/actionlint/cmd/actionlint
WALDEN_PKG := github.com/andrearaponi/walden/cmd/walden

GO_VERSION := $(shell awk '/^go /{print $$2; exit}' go.mod)
TOOLS_GOTOOLCHAIN := go$(GO_VERSION)

GO_RUN_TOOL = GOTOOLCHAIN=$(TOOLS_GOTOOLCHAIN) go run
GOLANGCI_LINT = $(GO_RUN_TOOL) $(GOLANGCI_LINT_PKG)@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK = $(GO_RUN_TOOL) $(GOVULNCHECK_PKG)@$(GOVULNCHECK_VERSION)
ACTIONLINT = $(GO_RUN_TOOL) $(ACTIONLINT_PKG)@$(ACTIONLINT_VERSION)
WALDEN = $(GO_RUN_TOOL) $(WALDEN_PKG)@$(WALDEN_VERSION)

# Scripts under scripts/ci/ build their tool commands from these variables.
export GO_VERSION TOOLS_GOTOOLCHAIN
export GOLANGCI_LINT_VERSION GOVULNCHECK_VERSION ACTIONLINT_VERSION WALDEN_VERSION
export GOLANGCI_LINT_PKG GOVULNCHECK_PKG ACTIONLINT_PKG WALDEN_PKG
