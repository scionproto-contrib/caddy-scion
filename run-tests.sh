#!/bin/bash

# Test Automation Script for caddy-scion
# Runs E2E and integration tests with automated SCION setup/teardown

set -eo pipefail

# Script directory and repository root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Global variables for cleanup
FORWARD_PID=""
REVERSE_PID=""
CREATED_ENV_FILE=false
ENV_FILE_PATH="/tmp/caddy-scion-environment-$$.json"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

#============================================
# Helper Functions
#============================================

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_success() {
    echo -e "${GREEN}✓${NC} $1"
}

log_failure() {
    echo -e "${RED}✗${NC} $1"
}

#============================================
# Setup Functions
#============================================

verify_prerequisites() {
    log_info "Verifying prerequisites..."
    local ERRORS=0

    # Check SCION directory
    if [ ! -d "$HOME/scion" ]; then
        log_failure "SCION directory not found at $HOME/scion"
        ERRORS=1
    else
        log_success "SCION directory exists"
    fi

    # Check scion.sh script
    if [ ! -x "$HOME/scion/scion.sh" ]; then
        log_failure "scion.sh not found or not executable at $HOME/scion/scion.sh"
        ERRORS=1
    else
        log_success "scion.sh is executable"
    fi

    # Check build directory and binaries
    if [ ! -d "./build" ]; then
        log_failure "Build directory not found. Run 'make build' first."
        ERRORS=1
    else
        log_success "Build directory exists"

        if [ ! -f "./build/scion-caddy-forward" ]; then
            log_failure "scion-caddy-forward binary not found. Run 'make build' first."
            ERRORS=1
        else
            log_success "scion-caddy-forward binary exists"
        fi

        if [ ! -f "./build/scion-caddy-reverse" ]; then
            log_failure "scion-caddy-reverse binary not found. Run 'make build' first."
            ERRORS=1
        else
            log_success "scion-caddy-reverse binary exists"
        fi
    fi

    # Check /etc/hosts configuration
    if ! grep -q "127.0.0.1.*scion.local" /etc/hosts; then
        log_warn "/etc/hosts missing 'scion.local' entry"
        echo "  Add this line: 127.0.0.1 scion.local"
        echo "  Run: sudo bash -c 'echo \"127.0.0.1 scion.local\" >> /etc/hosts'"
    else
        log_success "/etc/hosts contains scion.local entry"
    fi

    # Check /etc/scion/hosts configuration
    if [ ! -f /etc/scion/hosts ]; then
        log_warn "/etc/scion/hosts does not exist"
        echo "  Create with: sudo mkdir -p /etc/scion"
        echo "  Run: sudo bash -c 'echo \"1-ff00:0:112,[127.0.0.1] scion.local\" > /etc/scion/hosts'"
    elif ! grep -q "1-ff00:0:112,\[127.0.0.1\].*scion.local" /etc/scion/hosts; then
        log_warn "/etc/scion/hosts missing SCION address entry"
        echo "  Add this line: 1-ff00:0:112,[127.0.0.1] scion.local"
        echo "  Run: sudo bash -c 'echo \"1-ff00:0:112,[127.0.0.1] scion.local\" >> /etc/scion/hosts'"
    else
        log_success "/etc/scion/hosts contains SCION address entry"
    fi

    # Check Docker (for integration test)
    if ! command -v docker &> /dev/null; then
        log_warn "Docker not found. Integration tests will fail."
        echo "  Install Docker to run integration tests."
    else
        log_success "Docker is available"
    fi

    if [ $ERRORS -ne 0 ]; then
        log_error "Prerequisites check failed. Please fix the errors above."
        exit 1
    fi

    log_success "All prerequisites verified"
    echo ""
}

setup_environment() {
    log_info "Setting up SCION environment..."

    # Check if SCION_ENV_FILE is already set
    if [ -n "$SCION_ENV_FILE" ] && [ -f "$SCION_ENV_FILE" ]; then
        log_success "Using existing SCION_ENV_FILE: $SCION_ENV_FILE"
    else
        # Create temporary environment file
        log_info "Creating temporary environment file..."
        cat > "$ENV_FILE_PATH" <<EOF
{
    "ases": {
        "1-ff00:0:112": {
            "daemon_address": "127.0.0.27:30255"
        }
    }
}
EOF
        export SCION_ENV_FILE="$ENV_FILE_PATH"
        CREATED_ENV_FILE=true
        log_success "Created temporary environment file: $ENV_FILE_PATH"
    fi

    # Validate the environment file
    if ! python3 -m json.tool "$SCION_ENV_FILE" > /dev/null 2>&1; then
        log_error "Invalid JSON in SCION_ENV_FILE: $SCION_ENV_FILE"
        exit 1
    fi
    log_success "Environment file is valid JSON"
    echo ""
}

setup_scion() {
    log_info "Starting SCION topology and services..."

    cd "$HOME/scion"

    # Create topology
    log_info "Creating SCION topology (tiny4.topo)..."
    if ! ./scion.sh topology -c topology/tiny4.topo; then
        log_error "Failed to create SCION topology"
        cd "$SCRIPT_DIR"
        exit 1
    fi
    log_success "SCION topology created"

    # Start SCION services
    log_info "Starting SCION services..."
    if ! ./scion.sh run; then
        log_error "Failed to start SCION services"
        cd "$SCRIPT_DIR"
        exit 1
    fi
    log_success "SCION services started"

    cd "$SCRIPT_DIR"

    # Wait for services to be ready
    log_info "Waiting for SCION daemons to be ready..."
    sleep 5

    # Verify daemon connectivity
    if command -v scion &> /dev/null; then
        if scion showpaths 1-ff00:0:110 --sciond 127.0.0.27:30255 > /dev/null 2>&1; then
            log_success "SCION daemon is responding"
        else
            log_warn "SCION daemon connectivity check failed (but continuing...)"
        fi
    else
        log_warn "scion CLI not found, skipping daemon connectivity check"
    fi

    echo ""
}

#============================================
# Teardown Functions
#============================================

teardown_scion() {
    log_info "Stopping SCION services..."

    cd "$HOME/scion"

    # Stop SCION services
    if [ -f "./scion.sh" ]; then
        if ./scion.sh stop; then
            log_success "SCION services stopped"
        else
            log_warn "Failed to stop SCION services (but continuing...)"
        fi
    else
        log_warn "scion.sh not found, skipping SCION stop"
    fi

    # Clean up SCION logs and cache
    rm -rf logs/* 2>/dev/null || true
    rm -rf gen-cache/* 2>/dev/null || true

    cd "$SCRIPT_DIR"
    echo ""
}

cleanup_integration() {
    if [ -n "$FORWARD_PID" ] && kill -0 "$FORWARD_PID" 2>/dev/null; then
        log_info "Stopping forward proxy (PID: $FORWARD_PID)..."
        kill "$FORWARD_PID" 2>/dev/null || true
        sleep 1
        kill -9 "$FORWARD_PID" 2>/dev/null || true
    fi

    if [ -n "$REVERSE_PID" ] && kill -0 "$REVERSE_PID" 2>/dev/null; then
        log_info "Stopping reverse proxy (PID: $REVERSE_PID)..."
        kill "$REVERSE_PID" 2>/dev/null || true
        sleep 1
        kill -9 "$REVERSE_PID" 2>/dev/null || true
    fi

    # Stop Docker container if running
    if docker ps -q -f name=whoami | grep -q .; then
        log_info "Stopping whoami Docker container..."
        docker stop whoami > /dev/null 2>&1 || true
    fi

    # Clean up log files
    rm -f /tmp/forward-proxy.log /tmp/reverse-proxy.log

    # Clean up temporary certificate directory
    if [ -d "/tmp/caddy-scion-test" ]; then
        rm -rf /tmp/caddy-scion-test
    fi
}

cleanup_all() {
    echo ""
    log_info "Cleaning up..."

    cleanup_integration
    teardown_scion

    # Remove temporary environment file if we created it
    if [ "$CREATED_ENV_FILE" = true ] && [ -f "$ENV_FILE_PATH" ]; then
        rm -f "$ENV_FILE_PATH"
        log_success "Removed temporary environment file"
    fi

    log_success "Cleanup complete"
}

#============================================
# Test Functions
#============================================

run_e2e_test() {
    echo "========================================="
    echo "Running E2E Test"
    echo "========================================="
    echo ""

    # Run Go E2E test with proper flags
    log_info "Executing go test with e2e tag..."
    set +e  # Don't exit on test failure
    SCION_ENV_FILE="${SCION_ENV_FILE}" go test \
        -timeout 30s \
        -tags=e2e \
        -v \
        github.com/scionproto-contrib/caddy-scion/test \
        -sciond-address 127.0.0.19:30255

    E2E_RESULT=$?
    set -e

    echo ""
    if [ $E2E_RESULT -eq 0 ]; then
        log_success "E2E test PASSED"
    else
        log_failure "E2E test FAILED"
    fi
    echo ""

    return $E2E_RESULT
}

run_integration_test() {
    echo "========================================="
    echo "Running Integration Test"
    echo "========================================="
    echo ""

    # Step 0: Create temporary directory for test certificates
    log_info "Creating temporary directory for test certificates..."
    TEST_CERT_DIR="/tmp/caddy-scion-test"
    rm -rf "$TEST_CERT_DIR"  # Clean up any previous test data
    mkdir -p "$TEST_CERT_DIR"
    log_success "Temporary certificate directory created: $TEST_CERT_DIR"

    # Step 1: Start whoami Docker container
    log_info "Starting whoami container..."
    if ! docker run -p 8081:80 --name whoami --rm --detach traefik/whoami -verbose > /dev/null; then
        log_failure "Failed to start whoami container"
        return 1
    fi

    # Wait for container to be ready
    sleep 2

    # Verify whoami is responding
    set +e
    curl -s http://localhost:8081 > /dev/null
    CURL_RESULT=$?
    set -e

    if [ $CURL_RESULT -ne 0 ]; then
        log_failure "whoami container not responding on http://localhost:8081"
        docker stop whoami > /dev/null 2>&1 || true
        return 1
    fi
    log_success "whoami container started and responding"

    # Step 2: Start forward proxy in background (using test config)
    log_info "Starting forward proxy..."
    export SCION_DAEMON_ADDRESS="127.0.0.19:30255"
    ./build/scion-caddy-forward run --config ./test/configs/forward.json > /tmp/forward-proxy.log 2>&1 &
    FORWARD_PID=$!

    # Wait for forward proxy to be ready
    sleep 3
    if ! kill -0 $FORWARD_PID 2>/dev/null; then
        log_failure "Forward proxy failed to start"
        echo "Forward proxy logs:"
        cat /tmp/forward-proxy.log
        docker stop whoami > /dev/null 2>&1 || true
        return 1
    fi
    log_success "Forward proxy started (PID: $FORWARD_PID)"

    # Step 3: Start reverse proxy in background
    log_info "Starting reverse proxy..."
    unset SCION_DAEMON_ADDRESS  # Use default from environment.json
    ./build/scion-caddy-reverse run --config ./test/configs/reverse.json > /tmp/reverse-proxy.log 2>&1 &
    REVERSE_PID=$!

    # Wait for reverse proxy to be ready
    sleep 3
    if ! kill -0 $REVERSE_PID 2>/dev/null; then
        log_failure "Reverse proxy failed to start"
        echo "Reverse proxy logs:"
        cat /tmp/reverse-proxy.log
        kill $FORWARD_PID 2>/dev/null || true
        docker stop whoami > /dev/null 2>&1 || true
        return 1
    fi
    log_success "Reverse proxy started (PID: $REVERSE_PID)"

    # Give proxies a bit more time to fully initialize
    sleep 2

    # Step 4: Run test requests
    echo ""
    log_info "Testing HTTP over SCION (port 7080)..."
    set +e
    CURL_OUTPUT=$(curl -v "http://scion.local:7080" \
        --max-time 10 \
        --proxy "https://localhost:9443" \
        --proxy-insecure \
        --proxy-header "Proxy-Authorization: Basic $(echo -n 'policy:' | base64)" \
        2>&1)
    CURL_RESULT=$?
    set -e

    if [ $CURL_RESULT -eq 0 ] && echo "$CURL_OUTPUT" | grep -q "200\|HTTP/1.1 200"; then
        log_success "HTTP request via port 7080 PASSED"
        HTTP_TEST_7080=0
    else
        log_failure "HTTP request via port 7080 FAILED"
        echo "Curl output:"
        echo "$CURL_OUTPUT"
        HTTP_TEST_7080=1
    fi

    echo ""
    log_info "Testing HTTPS over SCION (port 7443)..."
    set +e
    CURL_OUTPUT=$(curl -v "https://scion.local:7443" \
        --max-time 10 \
        --insecure \
        --proxy "https://localhost:9443" \
        --proxy-insecure \
        --proxy-header "Proxy-Authorization: Basic $(echo -n 'policy:' | base64)" \
        2>&1)
    CURL_RESULT=$?
    set -e

    if [ $CURL_RESULT -eq 0 ] && echo "$CURL_OUTPUT" | grep -q "200\|HTTP/2 200\|HTTP/3 200"; then
        log_success "HTTPS request via port 7443 PASSED"
        HTTP_TEST_7443=0
    else
        log_failure "HTTPS request via port 7443 FAILED"
        echo "Curl output:"
        echo "$CURL_OUTPUT"
        HTTP_TEST_7443=1
    fi

    # Step 5: Cleanup integration test resources
    echo ""
    log_info "Cleaning up integration test resources..."
    kill $FORWARD_PID 2>/dev/null || true
    kill $REVERSE_PID 2>/dev/null || true
    docker stop whoami > /dev/null 2>&1 || true
    FORWARD_PID=""
    REVERSE_PID=""

    # Determine overall result
    echo ""
    if [ $HTTP_TEST_7443 -eq 0 ] && [ $HTTP_TEST_7080 -eq 0 ]; then
        log_success "Integration test PASSED"
        return 0
    else
        log_failure "Integration test FAILED"
        echo ""
        echo "Forward proxy logs:"
        cat /tmp/forward-proxy.log
        echo ""
        echo "Reverse proxy logs:"
        cat /tmp/reverse-proxy.log
        return 1
    fi
}

#============================================
# Main Control Logic
#============================================

# Parse command-line arguments
TEST_MODE="${1:-all}"  # Default: run all tests

# Display usage if invalid argument
if [[ "$TEST_MODE" != "e2e" && "$TEST_MODE" != "integration" && "$TEST_MODE" != "all" ]]; then
    echo "Usage: $0 [e2e|integration|all]"
    echo ""
    echo "Examples:"
    echo "  $0           # Run both tests (default)"
    echo "  $0 e2e       # Run E2E test only"
    echo "  $0 integration   # Run integration test only"
    exit 1
fi

# Trap for cleanup on exit (only set after argument validation)
trap cleanup_all EXIT INT TERM

# Main execution
echo "========================================="
echo "caddy-scion Test Automation"
echo "========================================="
echo ""

case "$TEST_MODE" in
    e2e)
        log_info "Running E2E test only"
        echo ""
        verify_prerequisites
        setup_environment
        setup_scion
        run_e2e_test
        EXIT_CODE=$?
        ;;

    integration)
        log_info "Running integration test only"
        echo ""
        verify_prerequisites
        setup_environment
        setup_scion
        run_integration_test
        EXIT_CODE=$?
        ;;

    all)
        log_info "Running all tests"
        echo ""
        verify_prerequisites
        setup_environment
        setup_scion

        # Run E2E test
        run_e2e_test
        E2E_EXIT=$?

        # Run integration test
        run_integration_test
        INTEGRATION_EXIT=$?

        # Report overall results
        echo ""
        echo "========================================="
        echo "Test Results Summary"
        echo "========================================="
        if [ $E2E_EXIT -eq 0 ]; then
            log_success "E2E Test: PASSED"
        else
            log_failure "E2E Test: FAILED"
        fi

        if [ $INTEGRATION_EXIT -eq 0 ]; then
            log_success "Integration Test: PASSED"
        else
            log_failure "Integration Test: FAILED"
        fi
        echo ""

        # Exit with failure if any test failed
        if [ $E2E_EXIT -eq 0 ] && [ $INTEGRATION_EXIT -eq 0 ]; then
            EXIT_CODE=0
            log_success "All tests PASSED!"
        else
            EXIT_CODE=1
            log_failure "Some tests FAILED"
        fi
        ;;
esac

exit $EXIT_CODE
