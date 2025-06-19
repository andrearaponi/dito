# Dito Deployments

This directory contains deployment configurations for various platforms and environments.

## Directory Structure

```
deployments/
├── kubernetes/          # Pure Kubernetes deployments
├── openshift/          # OpenShift-specific deployments  
└── docker/             # Docker and Docker Compose deployments
```

## Deployment Types

### Kubernetes (`kubernetes/`)
- **`basic-deployment.yaml`** - Simple Kubernetes deployment for development/testing
- Suitable for: Development, basic Kubernetes clusters

### OpenShift (`openshift/`)
- **`production-deployment.yaml`** - Production-ready OpenShift deployment
- Features: Plugin signing, security contexts, health checks
- Suitable for: Production OpenShift environments

### Docker (`docker/`)
- **`docker-compose.yml`** - Local development with Docker Compose  
- Suitable for: Local development, testing

## Quick Start

### OpenShift Production Deployment
```bash
# Deploy to OpenShift
make deploy-ocp

# Manual deployment
oc apply -f deployments/openshift/production-deployment.yaml
```

### Kubernetes Basic Deployment  
```bash
# Apply basic deployment
kubectl apply -f deployments/kubernetes/basic-deployment.yaml
```

### Local Docker Development
```bash
# Start with Docker Compose
cd deployments/docker
docker-compose up -d
```

## Configuration

Configuration templates and generated configs are located in the `configs/` directory.
