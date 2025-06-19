# Docker Deployments

Local development using Docker and Docker Compose.

## Files

### `docker-compose.yml`
Local development environment with:
- **Dito service** configuration
- **Volume mounts** for development
- **Port mapping** to localhost:8081
- **Health checks** enabled

## Prerequisites

- Docker and Docker Compose installed
- Dito built locally (`make build`)

## Usage

### Start Development Environment
```bash
# Build and start
cd deployments/docker
docker-compose up -d

# View logs
docker-compose logs -f dito

# Stop
docker-compose down
```

### Build and Run
```bash
# Build locally first
make build

# Start development environment
cd deployments/docker
docker-compose up
```

## Features

- **Local development** optimized
- **Fast iteration** with volume mounts
- **Health monitoring** with Docker health checks
- **Easy debugging** with direct log access

## Development Workflow

1. Make code changes
2. Run `make build` to rebuild binary
3. Restart container: `docker-compose restart dito`
4. Test changes at `http://localhost:8081`

## Accessing the Application

- **Metrics**: http://localhost:8081/metrics
- **Health**: Container health check automatic
- **Proxy endpoints**: Based on configuration
