# Kubernetes Deployments

Basic Kubernetes deployments for development and testing.

## Files

### `basic-deployment.yaml`
Simple Kubernetes deployment with:
- **Basic deployment** configuration
- **Service** definition
- **Minimal security** settings
- Suitable for development and testing

## Prerequisites

- Kubernetes cluster
- kubectl configured

## Deployment

```bash
# Apply basic deployment
kubectl apply -f deployments/kubernetes/basic-deployment.yaml

# Check status
kubectl get deployment dito
kubectl get service dito

# Port forward for testing
kubectl port-forward service/dito 8081:8081
```

## Features

- **Simple setup** for quick testing
- **Service exposure** on port 8081
- **Basic security** context
- **Resource requests** defined

## Notes

This is a basic deployment suitable for:
- Development environments
- Testing and experimentation
- Learning Kubernetes basics

For production deployments, use the OpenShift configuration which includes additional security and operational features.
