# Dito Configuration Files

This directory contains configuration templates and generated configuration files.

## Directory Structure

```
configs/
├── templates/          # Configuration templates
└── *.yaml             # Generated configuration files
```

## Templates (`templates/`)

### `application.yaml`
Base application configuration template with placeholders for:
- Plugin public key hash (`PLACEHOLDER_HASH_TO_BE_REPLACED`)
- Environment-specific paths
- Plugin directory configuration

## Generated Configs

### `config-prod-k8s.yaml`
Kubernetes/OpenShift-specific configuration generated from `templates/application.yaml`:
- Plugin directory: `./plugins`
- Public key path: `/app/keys/ed25519_public.key`
- Hash: Generated from production public key

## Usage

### Generate Kubernetes Config
```bash
# Generate config for Kubernetes deployment
make update-k8s-config
```

### Manual Template Processing
```bash
# Replace hash placeholder in template
HASH=$(shasum -a 256 bin/ed25519_public_prod.key | awk '{print $1}')
sed "s/PLACEHOLDER_HASH_TO_BE_REPLACED/$HASH/" configs/templates/application.yaml > configs/config-prod-k8s.yaml
```

## Best Practices

1. **Never commit generated configs** - Only commit templates
2. **Use templates for consistency** - All environments should derive from templates
3. **Environment-specific paths** - Adjust paths based on deployment target
4. **Security** - Never include secrets or keys in configuration files
