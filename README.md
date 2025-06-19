**Note:** The primary documentation for Dito will be migrated to an mdBook format. Please be patient 🙂

---

<div align="center">
  <h1>Dito</h1>
  <p>
    <img src="https://img.shields.io/badge/status-active-green.svg" alt="Status">
    <img src="https://img.shields.io/badge/release-0.7.5-green.svg" alt="Release">
    <img src="https://img.shields.io/badge/license-Apache2-blue.svg" alt="License">
    <img src="https://img.shields.io/badge/language-Go-blue.svg" alt="Language">
  </p>
  <p>
    <img src="dito.png" alt="Dito Logo">
  </p>
</div>

Dito is an advanced, highly extensible reverse proxy server written in Go. It features a robust plugin-based architecture, custom certificate handling for backend connections, dynamic configuration reloading, and more. Plugins can manage their own dependencies and provide custom middleware functionality.

## 🚀 Features

*   **Layer 7 Reverse Proxy:** Handles HTTP and HTTPS requests efficiently.
*   **WebSockets Support:** Proxy WebSocket connections with ease.
*   **Dynamic Configuration Reloading (hot reload):** Update configurations without restarting the server.
*   **Extensible Plugin System:** Enhance Dito’s functionality with custom Go plugins that can:
    *   Add authentication mechanisms
    *   Implement caching strategies
    *   Apply rate limiting
    *   Transform requests/responses
    *   Add custom logging
    *   And much more!
*   **Plugin Security:** Plugins are signed using Ed25519 keys, and Dito verifies these signatures at startup.
*   **Custom TLS Certificate Management:** Support for mTLS and custom certificates for backend connections.
*   **Header Manipulation:** Add or remove HTTP headers as needed.
*   **Advanced Logging:** Asynchronous logging with customizable verbosity and performance optimizations.
*   **Custom Transport Configuration:** Fine-tune HTTP transport settings per location or globally.
*   **Response Body Size Limits:** Control maximum response body sizes globally and per location with proper error handling (413 status code).
*   **Response Buffering Control:** Enable or disable response buffering per location for optimal performance.
*   **Prometheus Metrics:** Monitor performance and behavior with detailed metrics.

## 📂 Project Structure

```
dito/
├── cmd/                   # Entry points (main application & plugin-signer)
├── app/                   # Core application logic
├── config/                # Configuration loader & hot-reload
├── handlers/              # Request routing & proxy handlers
├── middlewares/           # Built-in middlewares (plugins can add more)
├── plugin/                # Plugin loading, signing, and verification
├── plugins/               # Plugin implementations
├── transport/             # HTTP transport configuration
├── websocket/             # WebSocket proxy support
├── writer/                # Response writers and buffering
├── metrics/               # Prometheus metrics
├── logging/               # Structured logging
├── deployments/           # Deployment configurations
│   ├── kubernetes/        # Basic Kubernetes deployments
│   ├── openshift/         # OpenShift production deployments
│   └── docker/            # Docker Compose for development
├── configs/               # Configuration files and templates
│   └── templates/         # Configuration templates
├── scripts/               # Deployment and utility scripts
└── bin/                   # Built binaries and runtime files
```
├── plugins/               # Example and community plugins
├── transport/             # HTTP transport customization
├── websockets/            # WebSocket support
├── writer/                # Custom response writers
├── logging/               # Logging utilities
└── metrics/               # Prometheus metrics collection
```

## ⚙️ Installation

Ensure you have Go (>= 1.21) and `make` installed.

### Quick Start (Recommended)

```bash
# Clone repo
git clone https://github.com/andrearaponi/dito.git && cd dito

# One-command setup & start
make quick-start
```
This will:
1.  Build all binaries (Dito & plugin-signer)
2.  Generate Ed25519 keys
3.  Build & sign plugins
4.  Update config with correct paths & hashes
5.  Start the Dito server

### Step-by-Step

```bash
# 1. Clone repo
git clone https://github.com/andrearaponi/dito.git && cd dito

# 2. Setup (build, keys, plugins, config)
make setup

# 3. Start server
make run
```

### Makefile Commands

| Category           | Command               | Description                                           |
| :----------------- | :-------------------- | :---------------------------------------------------- |
| 🚀 **Quick**       | `quick-start`         | Clean, setup everything and start (recommended)       |
|                    | `setup`               | Full development setup (build, keys, plugins, config) |
|                    | `setup-prod`          | Full production setup (persistent keys, prod config)  |
|                    | `run`                 | Start the Dito server                                 |
|                    | `fix-config`          | Quick command to fix configuration after setup        |
| 🔨 **Build**       | `build`               | Build Dito binary only                                |
|                    | `build-plugins`       | Build all plugins                                     |
|                    | `build-plugin-signer` | Build plugin-signer tool                              |
| 🔑 **Security**    | `generate-keys`       | Generate Ed25519 key pair for development             |
|                    | `generate-prod-keys`  | Generate persistent Ed25519 key pair for production   |
|                    | `sign-plugins`        | Sign all plugins with development keys                |
|                    | `sign-plugins-prod`   | Sign all plugins with production keys                 |
|                    | `update-config`       | Update bin/config.yaml with development key paths/hashes |
|                    | `update-prod-config`  | Update bin/config-prod.yaml with production key paths/hashes |
|                    | `update-k8s-config`   | Create configs/config-prod-k8s.yaml for Kubernetes deployment |
| 🚀 **OpenShift**   | `deploy-ocp`          | Complete OpenShift production deployment              |
|                    | `deploy-ocp-dev`      | Quick development deployment to OpenShift             |
|                    | `status-ocp`          | Check OpenShift deployment status                     |
|                    | `logs-ocp`            | View OpenShift deployment logs                        |
|                    | `clean-ocp`           | Clean up OpenShift resources                          |
| 🔍 **Debug**       | `debug-config`        | Debug configuration issues                            |
|                    | `help`                | Show all commands with detailed descriptions          |
| 🧹 **Cleanup**     | `clean`               | Remove all build artifacts                            |
|                    | `clean-plugins`       | Clean plugin binaries only                            |
| 🧪 **Development** | `test`                | Run tests                                             |
|                    | `vet`                 | Run go vet                                            |
|                    | `fmt`                 | Format code                                           |
|                    | `sonar`               | Run SonarQube analysis                                |

### 🛠️ Manual Installation (Advanced)

1.  **Build Dito:**
    ```bash
    go build -o bin/dito ./cmd/dito/main.go
    ```
    *(Note: Ensure the path to `main.go` is correct, e.g., `./cmd/dito/main.go` if `main.go` is in `cmd/dito/`)*

2.  **Build plugin-signer:**
    ```bash
    cd cmd/plugin-signer && go build -o ../../bin/plugin-signer . && cd ../..
    ```

3.  **Generate keys:**
    ```bash
    ./bin/plugin-signer generate-keys
    ```
    *(This will create keys in the current directory, presumably `bin/` if run from there, or the project root. Move them to `bin/` if needed or specify paths)*

4.  **Build plugins:**
    ```bash
    find plugins -mindepth 1 -maxdepth 1 -type d -exec sh -c 'cd "$1" && go build -buildmode=plugin -o "$(basename "$1").so"' sh {} \;
    ```

5.  **Sign plugins:**
    ```bash
    find plugins -name "*.so" -exec ./bin/plugin-signer sign {} \;
    ```
    *(Ensure `plugin-signer` can find the private keys; you might need to specify `-privateKey path/to/key`)*

6.  **Update `config.yaml`** (ensure `public_key_path` & `public_key_hash` are correct).

7.  **Run Dito:**
    ```bash
    ./bin/dito
    ```

## ⚙️ Usage

Start with default config:
```bash
make run
```

Or directly:
```bash
./bin/dito -f /path/to/custom-config.yaml -enable-profiler
```

### Config File
*   **Template:** `cmd/config.yaml` (or the correct path to your template)
*   **Runtime:** `bin/config.yaml` (auto-updated by `make setup` or `make quick-start`)

Key fields:
```yaml
port: '8081'
hot_reload: true
metrics:
  enabled: true
  path: "/metrics"
logging:
  enabled: true
  verbose: false
  level: "info"
plugins:
  directory: "./plugins" # Relative to where Dito is run, or absolute path
  public_key_path: "./ed25519_public.key" # Relative to where Dito is run, or absolute path
  public_key_hash: "<SHA256_HASH>" # The SHA256 hash of the public key
transport:
  http:
    idle_conn_timeout: 90s
    max_idle_conns: 1000
    max_idle_conns_per_host: 200
    max_conns_per_host: 0
    tls_handshake_timeout: 2s
    response_header_timeout: 2s
    expect_continue_timeout: 500ms
    disable_compression: false
    dial_timeout: 2s
    keep_alive: 30s
    force_http2: true
locations:
  - path: "^/test-ws$"
    target_url: "wss://echo.websocket.org"
    enable_websocket: true
    replace_path: true

  - path: "^/dito$"
    target_url: "https://httpbin.org/get"
    replace_path: true
    transport:
      http:
        disable_compression: true
    additional_headers:
      X-Custom: "true"
    excluded_headers:
      - Cookie
    middlewares:
      - hello-plugin # Plugin name as defined in its directory
```

## 📏 Response Limits Configuration

Dito provides flexible response body size limits that can be configured both globally and per location to prevent memory issues and control resource usage.

### Global Response Limits

Set default limits for all locations in the main configuration:

```yaml
# Global response limits configuration
response_limits:
  max_response_body_size: 100000 # 100KB default limit for all locations
```

### Per-Location Response Limits

Override global limits for specific locations with custom settings:

```yaml
locations:
  - path: "^/api/small"
    target_url: "https://api.example.com"
    max_response_body_size: 1024 # 1KB limit for this specific endpoint
    disable_response_buffering: false # Enable response buffering (default)
    
  - path: "^/api/large"
    target_url: "https://api.example.com"
    max_response_body_size: 52428800 # 50MB limit for large responses
    disable_response_buffering: true # Disable buffering for streaming
```

### Response Limit Features

- **Automatic Error Handling**: Returns proper `413 Request Entity Too Large` status code when limits are exceeded
- **JSON Error Responses**: Provides structured error messages with limit details
- **Early Detection**: Checks `Content-Length` header before processing to fail fast
- **Streaming Support**: Works with both buffered and unbuffered responses
- **Logging**: Comprehensive warning logs when limits are exceeded
- **Zero Downtime**: Limits can be updated via hot reload without server restart

### Error Response Format

When a response exceeds the configured limit, Dito returns a standardized JSON error:

```json
{
  "error": {
    "code": 413,
    "message": "Response body size exceeds limit",
    "details": {
      "limit_bytes": 90,
      "path": "/api/endpoint"
    }
  }
}
```

### Response Buffering Control

The `disable_response_buffering` option controls how responses are handled:

- **`false` (default)**: Responses are buffered in memory before sending to client
  - Better for small responses
  - Allows Content-Length to be set accurately
  - Enables proper error handling when limits are exceeded
  
- **`true`**: Responses are streamed directly to client  
  - Better for large responses or real-time data
  - Lower memory usage
  - Cannot recover if response exceeds limit mid-stream
## 🔌 Plugin System

Dito uses Go plugins (.so files). Each plugin must:
1.  Be in its own subdirectory under `plugins/`.
2.  Contain:
    *   `<plugin-name>.so`
    *   `<plugin-name>.so.sig` (signature file)
    *   `config.yaml` (plugin-specific config, optional but common)

### Signing & Verification
*   **Mandatory:** Dito will not start without valid plugin signing.
*   Uses Ed25519 digital signatures.

Steps (if done manually):
1.  **Generate key pair:**
    ```bash
    ./bin/plugin-signer generate-keys -privateKey ed25519_private.key -publicKey ed25519_public.key
    ```
    *(Save these keys securely, e.g., in `bin/` or a dedicated directory)*

2.  **Compute public key hash:**
    ```bash
    shasum -a 256 ed25519_public.key | awk '{print $1}'
    ```
    *(Ensure the path to `ed25519_public.key` is correct)*

3.  **Update Dito's `config.yaml`** with `public_key_path` (e.g., `./bin/ed25519_public.key`) and the computed `public_key_hash`.

4.  **Sign plugin:**
    ```bash
    ./bin/plugin-signer sign -plugin path/to/plugin.so -privateKey path/to/ed25519_private.key
    ```
    *(This will create a `.sig` file next to the plugin's `.so` file)*

## 🐛 Troubleshooting

*   **`public key integrity validation failed`**: Regenerate hash and update config. Ensure the hash exactly matches the specified public key file.
*   **`failed to read public key`**: Check public key file path in `config.yaml` & file permissions.
*   **`plugin signature verification failed`**: Re-sign with correct private key. Ensure public key in `config.yaml` matches the private key used for signing.


## Custom Transport Configuration

This allows for fine-grained control over how Dito connects to backend services, including:

- **Timeouts and Connection Limits**: Configure timeouts and maximum connections to handle backend service behavior.
- **TLS Settings**: Manage TLS handshake timeouts and enforce HTTP/2 if needed.
- **Custom Certificates**: Specify client certificates for mTLS connections to backends.

### WebSocket Support

Dito supports WebSocket proxying, allowing you to seamlessly forward WebSocket connections to your backend servers. This can be configured per location, enabling WebSocket support on specific routes.

#### Configuration

To enable WebSocket support, add `enable_websocket: true` to the location configuration. Here’s an example:

```yaml
# List of location configurations for proxying requests.
locations:
  - path: "^/test-ws$" # Regex pattern to match the request path.
    target_url: "wss://echo.websocket.org" # The target URL to which the request will be proxied.
    enable_websocket: true # Enable WebSocket support for this location.
    replace_path: true # Replace the matched path with the target URL.
```
#### Upcoming Enhancements

Future versions of Dito will include more advanced WebSocket features, such as:

- **Enhanced TLS Support**: Configurable TLS settings for secure WebSocket connections, allowing for encrypted communication and improved security.
- **Comprehensive Error Handling**: Improved resilience and error management for WebSocket connections to ensure stability during unexpected interruptions.
- **Detailed Metrics**: Real-time metrics for WebSocket traffic, enabling better performance monitoring and insight into connection stability and throughput.

These features aim to provide full control, security, and reliability for WebSocket connections in Dito, enhancing the overall communication experience.


### TLS/SSL

Dito supports mTLS (mutual TLS) for secure connections to backends. You can specify:

- `cert_file`: The client certificate.
- `key_file`: The client private key.
- `ca_file`: The certificate authority (CA) for verifying the backend.

## Metrics

Dito supports monitoring through Prometheus by exposing various metrics related to the proxy's performance and behavior. The metrics are accessible at the configured path (default is `/metrics`).

### Available Metrics

Dito provides both custom metrics and standard metrics from the Go runtime and Prometheus libraries:

#### Custom Metrics
- **`http_requests_total`**: Total number of HTTP requests processed, partitioned by method, path, and status code.
- **`http_request_duration_seconds`**: Duration of HTTP requests in seconds, with predefined buckets.
- **`active_connections`**: Number of active connections currently being handled by the proxy.
- **`data_transferred_bytes_total`**: Total amount of data transferred in bytes, partitioned by direction (`inbound` or `outbound`).

#### Standard Metrics
- **Go runtime metrics**: Metrics such as memory usage, garbage collection statistics, and the number of goroutines, which are automatically exposed by the Go Prometheus client library. Examples include:
   - `go_goroutines`: Number of goroutines currently running.
   - `go_memstats_alloc_bytes`: Number of bytes allocated in the heap.
   - `go_gc_duration_seconds`: Duration of garbage collection cycles.
- **Prometheus HTTP handler metrics**: Metrics related to the Prometheus HTTP handler itself, such as:
   - `promhttp_metric_handler_requests_total`: Total number of HTTP requests handled by the metrics endpoint.
   - `promhttp_metric_handler_requests_in_flight`: Current number of scrapes being served.

### Configuration Example

To enable metrics, make sure the following section is present in the `config.yaml` file:

```yaml
metrics:
   enabled: true # Enable or disable metrics.
   path: "/metrics" # The path on which the metrics will be exposed.
```
## Reporting Issues

If you encounter any issues while using Dito, please follow these steps to open an issue on the GitHub repository:

1. **Go Version**: Specify the version of Go you are using.
   - You can find your Go version by running `go version` in your terminal.

2. **Error Details**: Provide a detailed description of the error or issue you encountered. Include:
   - The exact error message.
   - The steps you took to produce the error.
   - Any relevant logs or console outputs.

3. **Configuration File**: Include the `config.yaml` file you are using.
   - This will help us understand the context in which the issue occurred and allow us to replicate the problem.

## 👏 Contributing

1.  Fork the repo.
2.  Create your feature branch (`git checkout -b feature/AmazingFeature`).
3.  Commit your changes (`git commit -m 'Add some AmazingFeature'`).
4.  Push to the branch (`git push origin feature/AmazingFeature`).
5.  Open a Pull Request.


## License

This project is licensed under the Apache License 2.0. See the [LICENSE](./LICENSE) file for details.

## 🐳 Containerization & OpenShift Deployment

Dito is fully containerized and optimized for OpenShift Container Platform (OCP) with enterprise-grade security and deployment practices.

### Quick Deployment

For a complete OpenShift deployment with automatic key management and configuration:

```bash
# Make the deployment script executable
chmod +x scripts/deploy-ocp.sh

# Deploy with defaults (namespace: dito, version: v2.0.0-production)
./scripts/deploy-ocp.sh

# Deploy to custom namespace with specific version
./scripts/deploy-ocp.sh -n my-dito -v latest

# Force key regeneration
./scripts/deploy-ocp.sh -f

# Only update configuration (no deployment)
./scripts/deploy-ocp.sh -c

# Only deploy (skip config/key generation)
./scripts/deploy-ocp.sh -d
```

### Manual Deployment Steps

#### 1. Build and Push Container Image

```bash
# Build for OpenShift with automatic registry login and push
./docker-build.sh

# Or with custom settings
VERSION=v2.1.0 NAMESPACE=my-dito ./docker-build.sh
```

#### 2. Generate Production Keys

```bash
# Generate persistent production keys (only if they don't exist)
make generate-prod-keys
```

#### 3. Create OpenShift Resources

```bash
# Create namespace
oc new-project dito

# Create Secret for keys
oc create secret generic dito-keys \
    --from-file=ed25519_public.key=bin/ed25519_public_prod.key \
    --from-file=ed25519_private.key=bin/ed25519_private_prod.key

# Create ConfigMap for application config
# First, create Kubernetes-specific config from template
HASH=$(shasum -a 256 bin/ed25519_public_prod.key | awk '{print $1}')
sed "s/PLACEHOLDER_HASH_TO_BE_REPLACED/$HASH/" configs/templates/application.yaml > configs/config-prod-k8s.yaml

oc create configmap dito-config \
    --from-file=config.yaml=configs/config-prod-k8s.yaml
```

#### 4. Deploy Application

```bash
# Deploy the application
oc apply -f deployments/openshift/production-deployment.yaml
```

### Deployment Models

| File | Use Case | Features |
|------|----------|----------|
| `deployments/kubernetes/basic-deployment.yaml` | Basic deployment | Simple setup, minimal security |
| `deployments/openshift/production-deployment.yaml` | Production deployment | Plugin signing, proper secrets, health checks |

### Security Features

#### Container Security
- **Non-root execution**: Runs as user ID 1001
- **Read-only root filesystem**: Prevents runtime modifications
- **Dropped capabilities**: Minimal privilege model
- **Security contexts**: OpenShift security context constraints

#### Key Management
- **External key generation**: Keys never stored in container images
- **Runtime plugin signing**: Plugins signed during pod initialization
- **Secure secret mounting**: Keys mounted as Kubernetes Secrets
- **Proper file permissions**: Keys accessible only to application user

### Configuration Management

Dito supports multiple configuration approaches:

1. **Template-based**: Use `configs/templates/application.yaml` with hash substitution
2. **Environment-specific**: Separate configs for dev/prod/staging in `configs/`
3. **ConfigMap injection**: Runtime configuration via Kubernetes ConfigMaps
4. **Hot-reload**: Dynamic configuration updates without restarts

See `configs/README.md` for detailed configuration management.
