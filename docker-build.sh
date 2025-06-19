#!/bin/bash

# Dito Docker Build Script for OpenShift Container Platform (OCP)
# This script builds and pushes Dito to an OCP internal registry

set -e

# Configuration
APP_NAME="dito"
VERSION="${VERSION:-latest}"
NAMESPACE="${NAMESPACE:-dito}"
OKD_REGISTRY="${OKD_REGISTRY:-default-route-openshift-image-registry.apps.okd4sno.okd.lan}"

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

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if ! command -v podman &> /dev/null && ! command -v docker &> /dev/null; then
        log_error "Neither podman nor docker found. Please install one of them."
        exit 1
    fi
    
    if ! command -v oc &> /dev/null; then
        log_error "OpenShift CLI (oc) not found. Please install it."
        exit 1
    fi
    
    # Prefer podman for OCP
    if command -v podman &> /dev/null; then
        CONTAINER_CMD="podman"
    else
        CONTAINER_CMD="docker"
        log_warning "Using Docker instead of Podman. Podman is recommended for OCP."
    fi
    
    log_success "Prerequisites check passed. Using $CONTAINER_CMD"
}

# Check OCP login
check_ocp_login() {
    log_info "Checking OpenShift login status..."
    
    if ! oc whoami &> /dev/null; then
        log_error "Not logged into OpenShift. Please run: oc login <cluster-url>"
        exit 1
    fi
    
    CURRENT_USER=$(oc whoami)
    log_success "Logged in as: $CURRENT_USER"
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

# Login to OCP registry
login_registry() {
    log_info "Logging into OCP registry: $OKD_REGISTRY"
    
    local token=$(oc whoami -t)
    if ! $CONTAINER_CMD login -u $(oc whoami) -p $token --tls-verify=false $OKD_REGISTRY; then
        log_error "Failed to login to OCP registry"
        exit 1
    fi
    
    log_success "Successfully logged into OCP registry"
}

# Build image
build_image() {
    log_info "Building Dito image for AMD64 platform..."
    
    local image_name="$APP_NAME:$VERSION"
    
    if ! $CONTAINER_CMD build --platform=linux/amd64 -t $image_name . --no-cache; then
        log_error "Failed to build image"
        exit 1
    fi
    
    log_success "Successfully built image: $image_name"
}

# Tag and push image
push_image() {
    log_info "Tagging and pushing image to OCP registry..."
    
    local local_image="$APP_NAME:$VERSION"
    local remote_image="$OKD_REGISTRY/$NAMESPACE/$APP_NAME:$VERSION"
    
    # Tag image for OCP registry
    if ! $CONTAINER_CMD tag $local_image $remote_image; then
        log_error "Failed to tag image"
        exit 1
    fi
    
    # Push to OCP registry
    if ! $CONTAINER_CMD push $remote_image; then
        log_error "Failed to push image to OCP registry"
        exit 1
    fi
    
    log_success "Successfully pushed image: $remote_image"
}

# Verify deployment
verify_deployment() {
    log_info "Verifying ImageStream creation..."
    
    sleep 2
    if oc get imagestream $APP_NAME -n $NAMESPACE &> /dev/null; then
        log_success "ImageStream created successfully"
        oc describe imagestream $APP_NAME -n $NAMESPACE
    else
        log_warning "ImageStream not found. It may take a moment to appear."
    fi
}

# Print deployment instructions
print_deployment_instructions() {
    log_info "Deployment Instructions:"
    
    cat << EOF

🚀 Your Dito image has been successfully pushed to the OCP registry!

📋 To deploy your application:

1. Create a deployment:
   oc create deployment $APP_NAME \\
     --image=image-registry.openshift-image-registry.svc:5000/$NAMESPACE/$APP_NAME:$VERSION

2. Expose the service:
   oc expose deployment $APP_NAME --port=8081
   oc expose service $APP_NAME

3. Update an existing deployment:
   oc set image deployment/$APP_NAME \\
     $APP_NAME=image-registry.openshift-image-registry.svc:5000/$NAMESPACE/$APP_NAME:$VERSION

4. Scale your deployment:
   oc scale deployment $APP_NAME --replicas=3

� For production deployments with secure key management:

1. Create secrets for Ed25519 keys:
   oc create secret generic dito-keys \\
     --from-file=ed25519_public.key=bin/ed25519_public.key \\
     --from-file=ed25519_private.key=bin/ed25519_private.key

2. Create configmap for configuration:
   oc create configmap dito-config --from-file=config.yaml=bin/config.yaml

3. Mount secrets and configmap in your deployment:
   - Mount dito-keys secret to /app/keys/
   - Mount dito-config configmap to /app/config/
   - Update config.yaml paths to use mounted volumes

�📊 Monitor your deployment:
   oc get pods -l app=$APP_NAME
   oc logs deployment/$APP_NAME

🔍 Access your application:
   oc get routes

EOF
}

# Main execution
main() {
    log_info "Starting Dito build and push process for OCP..."
    
    check_prerequisites
    check_ocp_login
    setup_namespace
    login_registry
    build_image
    push_image
    verify_deployment
    print_deployment_instructions
    
    log_success "🎉 Build and push completed successfully!"
}

# Help function
show_help() {
    cat << EOF
Dito Docker Build Script for OpenShift Container Platform

Usage: $0 [OPTIONS]

Options:
    -h, --help              Show this help message
    -v, --version VERSION   Set image version (default: latest)
    -n, --namespace NAME    Set target namespace (default: dito-prod)
    -r, --registry URL      Set OCP registry URL

Environment Variables:
    VERSION         Image version tag
    NAMESPACE       Target OCP namespace
    OKD_REGISTRY    OCP registry URL

Examples:
    $0                                          # Build with defaults
    $0 -v v0.7.5 -n production                # Build specific version
    VERSION=v0.7.5 NAMESPACE=prod $0           # Using environment variables

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -r|--registry)
            OKD_REGISTRY="$2"
            shift 2
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Run main function
main
echo "🐳 To run with docker-compose:"
echo "   docker-compose up -d"
