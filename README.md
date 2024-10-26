

<div align="center">
    <h1>Dito</h1>
    <img src="https://img.shields.io/badge/status-active-green.svg">
    <img src="https://img.shields.io/badge/release-0.4.1-green.svg">
    <img src="https://img.shields.io/badge/license-MIT-blue.svg">
    <img src="https://img.shields.io/badge/language-Go-blue.svg">
    <img src="dito.png" alt="Dito Logo" >
</div>


**Dito** is an advanced reverse proxy server written in Go. It provides flexible middleware support, custom certificate handling for backend connections, dynamic configuration reloading, and distributed caching and rate limiting with Redis.

## Features

- **Layer 7 Reverse Proxy**: Handles HTTP and HTTPS requests efficiently.
- **Dynamic Configuration Reloading** (`hot reload`): Update configurations without restarting the server.
- **Middleware Support**: Easily integrate custom middleware for authentication, rate limiting, caching, etc.
- **Distributed Rate Limiting with Redis**: Control request rates across multiple instances.
- **Distributed Caching with Redis**: Improve performance by caching responses.
- **Custom TLS Certificate Management**: Support for mTLS and custom certificates for backend connections.
- **Header Manipulation**: Add or remove HTTP headers as needed.
- **Advanced Logging**: Asynchronous logging with customizable verbosity and performance optimizations.
- **Custom Transport Configuration**: Fine-tune HTTP transport settings per location or globally.
- **Prometheus Metrics**: Monitor performance and behavior with detailed metrics.

## Project Structure

- `cmd/`: Entry point for the application.
- `app/`: Core application logic.
- `client`: Redis client for caching and rate limiting.
- `config/`: Configuration-related utilities (loading, hot-reload).
- `handlers/`: Core handlers for request routing and reverse proxy logic.
- `middlewares/`: Custom middleware implementations (e.g., authentication, caching, rate limiting).
- `transport/`: HTTP transport customization (including TLS management).
- `writer/`: Custom HTTP response writers for capturing status codes.
- `logging/`: Utilities for logging requests and responses.
- `metrics/`: Prometheus metrics collection and handling.

## Installation

Make sure you have Go installed (version 1.16 or later).

1. Clone the repository:

   ```bash
   git clone https://github.com/andrearaponi/dito.git
   cd dito
   ```

2. Build the application:

   ```bash
   go build -o dito ./cmd
   ```

## Usage

You can run Dito by simply executing the binary. By default, it looks for `config.yaml` in the current working directory.

```bash
./dito
```

### Command-Line Options

- `-f <path/to/config.yaml>`: Specify a custom configuration file.

Example:

```bash
./dito -f /path/to/custom-config.yaml
```

## Configuration

The configuration is defined in a `yaml` file, which can be dynamically reloaded if the `hot_reload` option is enabled. Here’s an example of a basic configuration:

```yaml
# Configuration for the proxy server.
port: '8081' # The port on which the server will listen.
hot_reload: true # Enable hot reloading of the configuration file.

# Logging configuration.
logging:
   enabled: true # Enable or disable logging.
   verbose: false # Enable or disable verbose logging.
   level: "info" # Set the log level (e.g., debug, info, warn, error)

# Metrics configuration.
metrics:
   enabled: true # Enable or disable metrics.
   path: "/metrics" # The path on which the metrics will be exposed.

# Redis configuration.
redis:
   enabled: true # Enable or disable Redis caching.
   host: "localhost"
   port: "6379"
   password: ""  # Leave empty if no password is required.
  
# Configuration for the HTTP transport settings.
transport:
  http:
    idle_conn_timeout: 90s  # The maximum amount of time an idle (keep-alive) connection will remain idle before closing.
    max_idle_conns: 1000  # The maximum number of idle (keep-alive) connections across all hosts.
    max_idle_conns_per_host: 200  # The maximum number of idle (keep-alive) connections to keep per-host.
    max_conns_per_host: 0  # The maximum number of connections per host. 0 means no limit.
    tls_handshake_timeout: 2s  # The maximum amount of time allowed for the TLS handshake.
    response_header_timeout: 2s  # The maximum amount of time to wait for a server's response headers after fully writing the request.
    expect_continue_timeout: 500ms  # The maximum amount of time to wait for a server's first response headers after fully writing the request headers if the request has an "Expect: 100-continue" header.
    disable_compression: false  # Whether to disable compression (gzip) for requests.
    dial_timeout: 2s  # The maximum amount of time to wait for a dial to complete.
    keep_alive: 30s  # The interval between keep-alive probes for an active network connection.
    force_http2: true  # Whether to force the use of HTTP/2.

# List of location configurations for proxying requests.
locations:
   - path: "^/dito$" # Regex pattern to match the request path.
     target_url: https://httpbin.org/get # The target URL to which the request will be proxied.
     replace_path: true # Replace the matched path with the target URL.
     
     # HTTP transport settings for this location. If not specified, the global settings will be used.
     transport:
       http:
         idle_conn_timeout: 90s  # The maximum amount of time an idle (keep-alive) connection will remain idle before closing.
         max_idle_conns: 1000  # The maximum number of idle (keep-alive) connections across all hosts.
         max_idle_conns_per_host: 400  # The maximum number of idle (keep-alive) connections to keep per-host.
         max_conns_per_host: 0  # The maximum number of connections per host. 0 means no limit.
         tls_handshake_timeout: 2s  # The maximum amount of time allowed for the TLS handshake.
         response_header_timeout: 2s  # The maximum amount of time to wait for a server's response headers after fully writing the request.
         expect_continue_timeout: 1s  # The maximum amount of time to wait for a server's first response headers after fully writing the request headers if the request has an "Expect: 100-continue" header.
         disable_compression: true  # Whether to disable compression (gzip) for requests.
         dial_timeout: 2s  # The maximum amount of time to wait for a dial to complete.
         keep_alive: 30s  # The interval between keep-alive probes for an active network connection.
         force_http2: false  # Whether to force the use of HTTP/2.
         cert_file: "" # Optional client certificate file for HTTPS connections.
         key_file: "" # Optional client key file for HTTPS connections.
         ca_file: "" # Optional CA certificate file for verifying server certificates.
     
     additional_headers:
        # Additional headers to be added to the request.
        il-molise: non esiste
     excluded_headers:
        - Cookie # Headers to be excluded from the request.
     middlewares:
        - rate-limiter-redis # List of middlewares to be applied.
        - cache
     rate_limiting:
        enabled: true
        requests_per_second: 2
        burst: 4
     cache:
        enabled: true
        ttl: 30
    
```

## Middlewares

Dito supports custom middlewares, which can be specified in the configuration. Currently available middleware includes:

- `auth`: Adds authentication logic.
- `rate-limiter`: Limits the number of requests per IP using an in-memory approach.
- `rate-limiter-redis`: Limits the number of requests per IP using Redis for distributed management.
- `cache`: Caches responses using Redis, improving performance for idempotent responses (e.g., GET).

### Middleware Execution Order

The order in which middlewares are applied is critical for ensuring proper functionality. For example:

- If the `rate-limiter` middleware is applied before `auth`, requests might be throttled before authentication is checked. This could lead to unauthorized requests consuming rate limit tokens.
- Similarly, applying `cache` before `auth` could result in cached responses being served to unauthenticated users, which is not ideal for secure endpoints.

#### Example Middleware Order:

1. `rate-limiter-redis`: Throttles requests to protect the backend.
2. `auth`: Ensures that only authenticated users can access the API.
3. `cache`: Caches responses to reduce load on backends for repeated requests.

To implement a new middleware, place your logic in the `middlewares/` directory and reference it in the configuration.


## Redis Integration

### Rate Limiting

Dito supports distributed rate limiting using Redis. The rate limiter can be configured per location with parameters like `requests_per_second` and `burst` to control the request flow.

### Caching

The `cache` middleware uses Redis to store responses. It helps in reducing load on backends by caching responses for a configurable `ttl` (time-to-live). The cache can be invalidated based on request headers or specific conditions.

### Implementing a New Middleware

To implement a new middleware, place your logic in the `middlewares/` directory and reference it in the configuration.

## Custom Transport Configuration

This allows for fine-grained control over how Dito connects to backend services, including:

- **Timeouts and Connection Limits**: Configure timeouts and maximum connections to handle backend service behavior.
- **TLS Settings**: Manage TLS handshake timeouts and enforce HTTP/2 if needed.
- **Custom Certificates**: Specify client certificates for mTLS connections to backends.


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

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
