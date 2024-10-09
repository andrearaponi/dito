

<div align="center">
    <h1>Dito</h1>
    <p>Advanced Layer 7 Reverse Proxy Server</p>
    <img src="https://img.shields.io/badge/status-active-green.svg">
    <img src="https://img.shields.io/badge/release-1.0.0-green.svg">
    <img src="https://img.shields.io/badge/license-MIT-blue.svg">
    <img src="https://img.shields.io/badge/language-Go-blue.svg">
    <img src="dito.png" alt="Dito Logo" >
</div>


**Dito** is an advanced Layer 7 reverse proxy server written in Go. It provides flexible middleware support, custom certificate handling for backend connections, dynamic configuration reloading, and distributed caching and rate limiting with Redis.

## Features

- Layer 7 reverse proxy for handling HTTP requests
- Dynamic configuration reloading (`hot reload`)
- Middleware support (e.g., an example of authentication, rate limiting, caching)
- Distributed rate limiting with Redis
- Distributed caching with Redis
- Custom TLS certificate management for backends (mTLS support)
- Header manipulation (additional headers, excluded headers)
- Logging support with detailed request and response logs

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
port: '8081'  # The port on which the server listens
hot_reload: true  # Enable hot-reload of configuration

logging:
  enabled: true  # Enable or disable logging
  verbose: false  # Enable verbose logging

redis:
  enabled: true  # Enable Redis for caching and rate limiting
  address: "localhost:6379"
  password: "yourpassword"
  db: 0

locations:
  - path: "^/api$"
    target_url: https://example.com
    replace_path: true
    additional_headers:
      X-Custom-Header: "my-value"
      il-molise: "non-esiste"
    excluded_headers:
      - Cookie
    middlewares:
      - auth
      - rate-limiter-redis
      - cache
    cache_config:
      enabled: true
      ttl: 60  # Cache time-to-live in seconds
    rate_limiting:
      enabled: true
      requests_per_second: 5
      burst: 10
    cert_file: "certs/client-cert.pem"
    key_file: "certs/client-key.pem"
    ca_file: "certs/ca-cert.pem"
```

## Middlewares

Dito supports custom middlewares, which can be specified in the configuration. Currently available middleware includes:

- `auth`: Adds authentication logic.
- `rate-limiter`: Limits the number of requests per IP using an in-memory approach.
- `rate-limiter-redis`: Limits the number of requests per IP using Redis for distributed management.
- `cache`: Caches responses using Redis, improving performance for idempotent responses (e.g., GET).

## Redis Integration

### Rate Limiting

Dito supports distributed rate limiting using Redis. The rate limiter can be configured per location with parameters like `requests_per_second` and `burst` to control the request flow.

### Caching

The `cache` middleware uses Redis to store responses. It helps in reducing load on backends by caching responses for a configurable `ttl` (time-to-live). The cache can be invalidated based on request headers or specific conditions.

### Implementing a New Middleware

To implement a new middleware, place your logic in the `middlewares/` directory and reference it in the configuration.

## TLS/SSL

Dito supports mTLS (mutual TLS) for secure connections to backends. You can specify:

- `cert_file`: The client certificate.
- `key_file`: The client private key.
- `ca_file`: The certificate authority (CA) for verifying the backend.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
