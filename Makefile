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

# SONAR_HOST_URL: The URL of the SonarQube server.
# SONAR_PROJECT_KEY: The unique key for the SonarQube project.
SONAR_HOST_URL=http://localhost:9000
SONAR_PROJECT_KEY=dito

# .PHONY: Declares phony targets that are not actual files.
.PHONY: build sonar test vet fmt clean run

# build: Compiles the Go project and copies the configuration file to the bin directory.
build:
	$(GO_BUILD) -o bin/$(BINARY_NAME) $(API_DIR)/*.go && cp $(CONFIG_FILE) bin/

# vet: Runs the Go vet tool.
vet:
	$(GO_VET) $(PKG)

# fmt: Formats the Go code.
fmt:
	$(GO_FMT) $(PKG)

# clean: Removes the binary and configuration file from the bin directory.
clean:
	rm -f bin/$(BINARY_NAME) && rm -f bin/config.yaml

# run: Runs the compiled binary.
run:
	 cd bin && ./$(BINARY_NAME)

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

