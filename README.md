**Note:** The primary documentation for Dito will be migrated to an mdBook format. Please be patient 🙂

---

<div align="center">
  <h1>Dito</h1>
  <p>
    <img src="https://img.shields.io/badge/status-active-green.svg" alt="Status">
    <img src="https://img.shields.io/badge/release-0.7.0-green.svg" alt="Release">
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

| Category        | Command               | Description                                           |
| :-------------- | :-------------------- | :---------------------------------------------------- |
| 🚀 **Quick**    | `quick-start`         | Clean, setup everything and start (recommended)       |
|                 | `setup`               | Full setup (build, keys, plugins, config)             |
|                 | `run`                 | Start the Dito server                                 |
| 🔨 **Build**    | `build`               | Build Dito binary only                                |
|                 | `build-plugins`       | Build all plugins                                     |
|                 | `build-plugin-signer` | Build plugin-signer tool                              |
| 🔑 **Security** | `generate-keys`       | Generate Ed25519 key pair                             |
|                 | `sign-plugins`        | Sign all plugins                                      |
|                 | `update-config`       | Update config paths & key hashes                      |
| 🔍 **Debug**    | `debug-config`        | Debug configuration issues                            |
|                 | `help`                | Show all commands                                     |
| 🧹 **Cleanup**  | `clean`               | Remove all build artifacts                            |
|                 | `clean-plugins`       | Clean plugin binaries only                            |
| 🧪 **Development**| `test`                | Run tests                                             |
|                 | `vet`                 | Run go vet                                            |
|                 | `fmt`                 | Format code                                           |

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
