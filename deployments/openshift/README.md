# OpenShift Deployments

Production-ready deployments for OpenShift Container Platform.

## Files

### `production-deployment.yaml`
Complete production deployment with:
- **Init container** for plugin signing at runtime
- **Security contexts** for non-root execution  
- **Health checks** (startup, liveness, readiness probes)
- **Resource limits** and requests
- **Secret mounting** for Ed25519 keys
- **ConfigMap mounting** for application configuration
- **Service** and **Route** definitions

## Prerequisites

1. **Keys**: Production Ed25519 keys must be generated
2. **ConfigMap**: Application configuration must be created
3. **Secret**: Ed25519 keys must be stored as Secret

## Deployment

### Automated (Recommended)
```bash
# Complete deployment with all prerequisites
make deploy-ocp

# Or with custom namespace/version
NAMESPACE=my-dito VERSION=v1.0.0 make deploy-ocp
```

### Manual
```bash
# 1. Generate keys and config
make update-k8s-config

# 2. Create Secret
oc create secret generic dito-keys \
  --from-file=ed25519_public.key=bin/ed25519_public_prod.key \
  --from-file=ed25519_private.key=bin/ed25519_private_prod.key

# 3. Create ConfigMap  
oc create configmap dito-config \
  --from-file=config.yaml=configs/config-prod-k8s.yaml

# 4. Deploy application
oc apply -f deployments/openshift/production-deployment.yaml
```

## Features

- **Runtime plugin signing** via init container
- **Non-root security** with user ID 1001
- **Resource management** with requests/limits
- **High availability** ready (can scale replicas)
- **OpenShift Route** for external access
- **Prometheus metrics** exposed at `/metrics`

## Monitoring

```bash
# Check deployment status
oc get deployment dito

# View logs
oc logs deployment/dito

# Port forward for testing
oc port-forward service/dito 8081:8081
```
