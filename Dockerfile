# Multi-stage build for Dito - OpenShift compatible (AMD64)
#
# Red Hat does not ship a Go 1.27 go-toolset yet, so the latest UBI 8 go-toolset
# is only used as a bootstrap: GOTOOLCHAIN makes the go command download the
# official go1.27.1 toolchain (verified against sum.golang.org). The builder stays
# on UBI 8 so that the CGO-enabled binaries (required by Go plugins) link against
# the same glibc as the ubi8-minimal runtime image.
# Host binary and plugins MUST be built with the exact same toolchain.
# Base images are pinned by tag and digest; Renovate proposes their updates.
FROM --platform=linux/amd64 registry.access.redhat.com/ubi8/go-toolset:1.26.7@sha256:1439433a2cd76f0c20035001d46074e85a59ccd0d318a16023c3fd9fdd18ddf5 AS builder

ENV GOTOOLCHAIN=go1.27.1 \
    GOPROXY=https://proxy.golang.org,direct \
    GOSUMDB=sum.golang.org

# Install build dependencies
USER 0
RUN dnf install -y git make gcc && dnf clean all

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled for plugins support and force AMD64 architecture
ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=amd64

# Only build binaries and unsigned plugins, NO key generation
RUN make build-plugin-signer build build-plugins

# Production image - OpenShift compatible (AMD64)
FROM --platform=linux/amd64 registry.access.redhat.com/ubi8/ubi-minimal:8.10@sha256:a2006dac3089997a79cb2d211a80ac59949d18f16e48bb89849fb96efabb970b

# Install runtime dependencies
RUN microdnf update -y && \
    microdnf install -y ca-certificates wget shadow-utils && \
    microdnf clean all && \
    rm -rf /var/cache/yum

# Create non-root user and group (OpenShift will override UID but not GID)
RUN groupadd -g 1001 dito && \
    useradd -r -u 1001 -g dito -s /bin/false -M dito

# Set working directory
WORKDIR /app

# Create all required directories upfront
RUN mkdir -p /app/plugins /app/logs /app/tmp /app/keys && \
    chown -R 1001:1001 /app

# Copy application files from builder
COPY --from=builder --chown=1001:1001 /app/bin/dito /app/dito
COPY --from=builder --chown=1001:1001 /app/bin/plugin-signer /app/plugin-signer
COPY --from=builder --chown=1001:1001 /app/cmd/config.yaml /app/config.yaml

# Copy plugins if they exist (handle empty directory gracefully)
# Using shell to handle potentially empty directory
RUN --mount=type=bind,from=builder,source=/app/plugins,target=/tmp/plugins \
    if [ -d "/tmp/plugins" ] && [ "$(ls -A /tmp/plugins 2>/dev/null)" ]; then \
        cp -r /tmp/plugins/* /app/plugins/ && \
        chown -R 1001:1001 /app/plugins; \
    else \
        echo "No plugins to copy"; \
    fi

# OpenShift compatibility - grant group permissions
# OpenShift runs with random UID but GID 0
RUN chgrp -R 0 /app && \
    chmod -R g=u /app && \
    chmod g+s /app

# Remove any setuid/setgid bits for security
RUN find /app -perm /6000 -type f -exec chmod a-s {} \; 2>/dev/null || true

# Switch to non-root user
USER 1001

# Expose port
EXPOSE 8081

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8081/metrics || exit 1

# Environment variables
ENV PORT=8081 \
    HOME=/app

# Run the application
ENTRYPOINT ["/app/dito"]
CMD ["-f", "/app/config.yaml"]