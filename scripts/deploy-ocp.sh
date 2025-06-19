#!/bin/bash

# Dito OpenShift Deployment Script
# This script automates the complete deployment process including:
# - Key generation and management
# - Config templating
# - Secret and ConfigMap creation
# - Application deployment

set -e

# Configuration
APP_NAME="dito"
VERSION="${VERSION:-v2.0.0-production}"
NAMESPACE="${NAMESPACE:-dito}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Usage information
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Dito OpenShift Deployment Script

OPTIONS:
    -n, --namespace <namespace>    OpenShift namespace (default: dito)
    -v, --version <version>        Image version (default: v2.0.0-production)
    -f, --force-keys              Force regeneration of production keys
    -c, --config-only             Only update configuration (no deployment)
    -d, --deploy-only             Only deploy (skip config/key generation)
    -h, --help                    Show this help message

EXAMPLES:
    $0                            # Full deployment with defaults
    $0 -n my-dito -v latest       # Deploy to 'my-dito' namespace with 'latest' tag
    $0 -f                         # Force key regeneration
    $0 -c                         # Only update configuration
    $0 -d                         # Only deploy (assuming keys/config are ready)

EOF
}

# Parse command line arguments
FORCE_KEYS=false
CONFIG_ONLY=false
DEPLOY_ONLY=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -f|--force-keys)
            FORCE_KEYS=true
            shift
            ;;
        -c|--config-only)
            CONFIG_ONLY=true
            shift
            ;;
        -d|--deploy-only)
            DEPLOY_ONLY=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if ! command -v oc &> /dev/null; then
        log_error "OpenShift CLI (oc) not found. Please install it."
        exit 1
    fi
    
    if ! command -v shasum &> /dev/null && ! command -v sha256sum &> /dev/null; then
        log_error "Neither shasum nor sha256sum found. Please install one of them."
        exit 1
    fi
    
    if ! oc whoami &> /dev/null; then
        log_error "Not logged into OpenShift. Please run: oc login <cluster-url>"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Setup namespace
setup_namespace() {
    log_info "Setting up namespace: $NAMESPACE"
    
    if ! oc project $NAMESPACE &> /dev/null; then
        log_info "Creating new project: $NAMESPACE"
        oc new-project $NAMESPACE
    else
        log_success "Using existing project: $NAMESPACE"
    fi
}

# Generate or verify production keys
manage_keys() {
    if [ "$DEPLOY_ONLY" = true ]; then
        log_info "Skipping key management (deploy-only mode)"
        return
    fi
    
    log_info "Managing production keys..."
    cd "$PROJECT_ROOT"
    
    if [ "$FORCE_KEYS" = true ] || [ ! -f "bin/ed25519_public_prod.key" ] || [ ! -f "bin/ed25519_private_prod.key" ]; then
        if [ "$FORCE_KEYS" = true ]; then
            log_warning "Force regenerating production keys..."
            rm -f bin/ed25519_public_prod.key bin/ed25519_private_prod.key
        else
            log_info "Production keys not found, generating..."
        fi
        
        make generate-prod-keys
        log_success "Production keys generated successfully"
    else
        log_success "Production keys already exist"
    fi
}

# Create OpenShift ConfigMap and Secret
create_k8s_resources() {
    if [ "$DEPLOY_ONLY" = true ]; then
        log_info "Skipping resource creation (deploy-only mode)"
        return
    fi
    
    log_info "Creating Kubernetes configuration..."
    cd "$PROJECT_ROOT"
    
    # Always regenerate config for Kubernetes to ensure it's correct
    log_info "Regenerating Kubernetes-specific configuration..."
    
    # Get the public key hash from the production key
    if command -v shasum >/dev/null 2>&1; then
        HASH=$(shasum -a 256 bin/ed25519_public_prod.key | awk '{print $1}')
    else
        HASH=$(sha256sum bin/ed25519_public_prod.key | awk '{print $1}')
    fi
    
    log_info "Public key hash: $HASH"
    
    # Create Kubernetes config from template with correct path
    sed "s/PLACEHOLDER_HASH_TO_BE_REPLACED/$HASH/" configs/templates/application.yaml > configs/config-prod-k8s.yaml
    log_success "Kubernetes config created: configs/config-prod-k8s.yaml"
    
    # Create Secret for keys
    log_info "Creating Secret for production keys..."
    oc delete secret dito-keys -n $NAMESPACE 2>/dev/null || true
    oc create secret generic dito-keys \
        --from-file=ed25519_public.key=bin/ed25519_public_prod.key \
        --from-file=ed25519_private.key=bin/ed25519_private_prod.key \
        -n $NAMESPACE
    log_success "Secret 'dito-keys' created"
    
    # Create ConfigMap for configuration
    log_info "Creating ConfigMap for configuration..."
    oc delete configmap dito-config -n $NAMESPACE 2>/dev/null || true
    oc create configmap dito-config \
        --from-file=config.yaml=configs/config-prod-k8s.yaml \
        -n $NAMESPACE
    log_success "ConfigMap 'dito-config' created"
}

# Deploy application
deploy_application() {
    if [ "$CONFIG_ONLY" = true ]; then
        log_info "Skipping deployment (config-only mode)"
        return
    fi
    
    log_info "Deploying Dito application..."
    cd "$PROJECT_ROOT"
    
    # Update deployment with correct image version
    local temp_deployment=$(mktemp)
    sed "s|image: image-registry.openshift-image-registry.svc:5000/dito/dito:.*|image: image-registry.openshift-image-registry.svc:5000/$NAMESPACE/$APP_NAME:$VERSION|g" \
        deployments/openshift/production-deployment.yaml > "$temp_deployment"
    
    # Apply deployment
    oc apply -f "$temp_deployment" -n $NAMESPACE
    rm "$temp_deployment"
    
    log_success "Deployment applied"
    
    # Wait for deployment to be ready
    log_info "Waiting for deployment to be ready..."
    oc rollout status deployment/dito -n $NAMESPACE --timeout=300s
    
    log_success "Deployment is ready!"
}

# Verify deployment
verify_deployment() {
    if [ "$CONFIG_ONLY" = true ]; then
        return
    fi
    
    log_info "Verifying deployment..."
    
    # Check pod status
    log_info "Pod status:"
    oc get pods -l app=dito -n $NAMESPACE
    
    # Check service
    log_info "Service status:"
    oc get svc dito -n $NAMESPACE
    
    # Check route
    log_info "Route status:"
    oc get route dito -n $NAMESPACE
    
    # Get route URL
    local route_url=$(oc get route dito -n $NAMESPACE -o jsonpath='{.spec.host}' 2>/dev/null || echo "Route not found")
    if [ "$route_url" != "Route not found" ]; then
        log_success "Dito is accessible at: https://$route_url"
        log_info "Health check: https://$route_url/metrics"
    fi
}

# Print summary
print_summary() {
    cat << EOF

🎉 Deployment Summary
====================

Namespace: $NAMESPACE
Version: $VERSION
Application: $APP_NAME

📁 Files created:
  - bin/ed25519_public_prod.key (production public key)
  - bin/ed25519_private_prod.key (production private key)
  - configs/config-prod-k8s.yaml (Kubernetes configuration)

🔒 Kubernetes Resources:
  - Secret: dito-keys (contains production keys)
  - ConfigMap: dito-config (contains application config)
  - Deployment: dito
  - Service: dito
  - Route: dito

🚀 Next Steps:
  - Monitor deployment: oc get pods -l app=dito -n $NAMESPACE
  - Check logs: oc logs -l app=dito -n $NAMESPACE
  - Access application via the route URL shown above

EOF
}

# Main execution
main() {
    log_info "Starting Dito OpenShift deployment..."
    log_info "Namespace: $NAMESPACE, Version: $VERSION"
    
    check_prerequisites
    setup_namespace
    manage_keys
    create_k8s_resources
    deploy_application
    verify_deployment
    print_summary
    
    log_success "Deployment completed successfully! 🎉"
}

# Run main function
main "$@"
