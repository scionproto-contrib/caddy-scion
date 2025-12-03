# Testing Guide for caddy-scion

This document describes how to run automated tests for the caddy-scion project.

## Operational note

The tests require a SCION environment to be set up and running. 
The test scripts handle starting and stopping SCION services automatically, but you must have a valid SCION installation and configuration on your host machine.

**NOTE** The script automatically cleans up the development SCION environment after the tests complete, which means that `scion/gen-cache/` and `scion/logs/` will be removed. If you have any important data in these directories, please back them up before running the tests.

## Quick Start

```bash
# 1. Build the binaries
make build

# 2. Run all tests (automatically checks host configuration)
make test
```

## Test Scripts

### Makefile Test Targets

The Makefile provides convenient test targets that wrap `run-tests.sh`:

**Available Targets:**
```bash
make test              # Run all tests (E2E + integration)
make test-e2e          # Run E2E tests only
make test-integration  # Run integration tests only
```

**Usage:**
```bash
# Run complete test suite
make test

# Run only E2E tests
make test-e2e

# Run only integration tests
make test-integration

# Build and test in one command
make
```

**What they do:**
- `make test` → `./run-tests.sh` (both E2E and integration)
- `make test-e2e` → `./run-tests.sh e2e`
- `make test-integration` → `./run-tests.sh integration`

All targets include automatic SCION setup/teardown and prerequisite validation.

### `run-tests.sh` - Main Test Automation Script

Automated test runner that handles SCION setup/teardown and runs both E2E and integration tests.

**Usage:**
```bash
./run-tests.sh [e2e|integration|all]
```

**Options:**
- `./run-tests.sh` or `./run-tests.sh all` - Run both E2E and integration tests (default)
- `./run-tests.sh e2e` - Run E2E test only
- `./run-tests.sh integration` - Run integration test only

**What it does:**
1. Verifies prerequisites (SCION directory, binaries, Docker, host configuration)
2. Creates temporary SCION environment file (if `SCION_ENV_FILE` not set)
3. Starts SCION topology (`tiny4.topo`) and services
4. Runs the selected test(s)
5. Cleans up all resources (SCION, proxies, Docker containers)

**E2E Test:**
- We verify end-to-end functionality of the Caddy-SCION plugins:
  - HTTPS target via HTTPS proxy over IP
  - HTTPS target via HTTPS proxy over SCION
  - HTTP target via HTTPS proxy over SCION
  - Direct HTTPS target over IP
  - HTTP/3 target over SCION
- Runs Go tests with `-tags=e2e`
- Tests forward and reverse proxies over SCION
- Uses programmatic configuration (no external config files)

**Integration Test:**
- It verifies basic functionality between forward and reverse proxies.
  - We do not try HTTP3 listener on the reverse proxy as it cannot be tunneled through HTTP CONNECT proxies.
- Starts Traefik whoami container (target server)
- Starts forward proxy (`scion-caddy-forward`) with test-specific TLS configuration
- Starts reverse proxy (`scion-caddy-reverse`)
- Tests HTTP requests through proxies:
  - HTTPS via port 7443 (single-stream SCION with TLS)
  - HTTP via port 7080 (single-stream SCION without TLS)
- Note: HTTP/3 (port 8443) is not tested as it cannot be tunneled through HTTP CONNECT proxies

## Prerequisites

### Required Software
- [Development Setup](https://scion-http-proxy.readthedocs.io/en/latest/dev_setup.html)

### Host Configuration

**Add to `/etc/hosts`:**
```
127.0.0.1 scion.local
```

**Command:**
```bash
sudo bash -c 'echo "127.0.0.1 scion.local" >> /etc/hosts'
```

**Add to `/etc/scion/hosts`:**
```
1-ff00:0:112,[127.0.0.1] scion.local
```

**Commands:**
```bash
sudo bash -c 'echo "1-ff00:0:112,[127.0.0.1] scion.local" > /etc/scion/hosts'
```

### Build Binaries

Before running tests, build the binaries:
```bash
make build
```

This creates:
- `./build/scion-caddy-forward`
- `./build/scion-caddy-reverse`
- `./build/scion-caddy-native`
- `./build/scion-caddy`

## Environment Variables

### `SCION_ENV_FILE`

Path to SCION environment configuration file. If not set, the script creates a temporary file with default configuration:

```json
{
    "ases": {
        "1-ff00:0:112": {
            "daemon_address": "127.0.0.27:30255"
        }
    }
}
```

**To use a custom environment file:**
```bash
export SCION_ENV_FILE="/path/to/environment.json"
./run-tests.sh
```

## Examples

### Run All Tests

**Using Makefile (recommended):**
```bash
make test
```

**Using script directly:**
```bash
./run-tests.sh
```

### Run Only E2E Test

**Using Makefile (recommended):**
```bash
make test-e2e
```

**Using script directly:**
```bash
./run-tests.sh e2e
```

### Run Only Integration Test

**Using Makefile (recommended):**
```bash
make test-integration
```

**Using script directly:**
```bash
./run-tests.sh integration
```

## Troubleshooting

### Test Fails with "SCION daemon not responding"

**Symptoms:**
- E2E test panics with "unable to connect to SCIOND"
- Integration test times out

**Solutions:**
1. Verify SCION is running: `cd ~/scion && ./scion.sh status`
2. Check daemon address matches environment file
3. Verify firewall allows local UDP connections

### Test Fails with "Binary not found"

**Symptoms:**
- Script exits with "scion-caddy-forward binary not found"

**Solution:**
```bash
make build
```

### Integration Test Fails with "whoami container not responding"

**Symptoms:**
- Container starts but curl fails

**Solutions:**
1. Check Docker is running: `docker ps`
2. Verify port 8081 is not already in use: `lsof -i :8081`
3. Check Docker logs: `docker logs whoami`

### Proxies Fail to Start

**Symptoms:**
- Script reports "Forward proxy failed to start"
- Or "Reverse proxy failed to start"

**Solutions:**
1. Check logs: `/tmp/forward-proxy.log` or `/tmp/reverse-proxy.log`
2. Verify ports are not in use:
   - Forward proxy: 9080, 9443
   - Reverse proxy: 7080, 7443, 8443
3. Check SCION daemon is accessible

### TLS Permission Errors

**Symptoms:**
- Forward proxy fails with PKI/TLS errors

**Solution:**
The test script automatically uses a separate configuration (`test/configs/forward.json`) that stores certificates in `/tmp/caddy-scion-test/` to avoid conflicts with running proxy instances. This directory is automatically created and cleaned up by the test script.

### "Host configuration has errors"

**Symptoms:**
- `run-tests.sh` reports missing `/etc/hosts` or `/etc/scion/hosts` entries

**Solution:**
Follow the instructions provided by the script to add missing entries. The script will display the exact commands needed to fix the configuration.

## Cleanup

The script automatically cleans up all resources on exit (success, failure, or interrupt):
- Stops SCION services
- Kills proxy processes
- Stops Docker containers
- Removes temporary files

**Manual cleanup (if needed):**
```bash
# Stop SCION
cd ~/scion
./scion.sh stop
rm -rf logs/*
rm -rf gen-cache/*

# Kill any leftover processes
pkill -f scion-caddy

# Remove Docker containers
docker stop whoami
```

## Test Configuration

### Forward Proxy Configuration
- **Test config:** `./test/configs/forward.json` (uses `/tmp/caddy-scion-test/`)
  - The test configuration avoids permission conflicts with running proxy instances
  - Automatically cleaned up after tests complete
- **Ports:** 9080 (HTTP), 9443 (HTTPS)
- **Hosts:** localhost, forward-proxy.scion
- **TLS:** Self-signed certificate (internal CA)

### Reverse Proxy Configuration
- **Config file:** `./test/configs/reverse.json`
- **Listeners:**
  - `scion+single-stream/[1-ff00:0:112,127.0.0.1]:7080` (HTTP)
  - `scion+single-stream/[1-ff00:0:112,127.0.0.1]:7443` (HTTPS)
  - `scion/[1-ff00:0:112,127.0.0.1]:8443` (HTTP/3)
- **Upstream:** localhost:8081 (whoami container)

### E2E Test Configuration
- **Programmatic configuration** (defined in `test/e2e_test.go`)
- **Tests multiple scenarios:**
  - HTTPS target via HTTPS proxy over IP
  - HTTPS target via HTTPS proxy over SCION
  - HTTP target via HTTPS proxy over SCION
  - Direct HTTPS target over IP
  - HTTP/3 target over SCION


## Advanced Usage

### Using a Different SCION Topology

Edit the script or set up your environment:
```bash
cd ~/scion
./scion.sh topology -c topology/your-topology.topo
./scion.sh run
```

Then run tests with existing SCION setup (you may need to modify the script to skip setup).

### Custom Environment Configuration

Create a custom environment file:
```bash
cat > /tmp/my-environment.json <<EOF
{
    "ases": {
        "1-ff00:0:112": {
            "daemon_address": "127.0.0.27:30255"
        },
        "1-ff00:0:111": {
            "daemon_address": "127.0.0.19:30255"
        }
    }
}
EOF

export SCION_ENV_FILE="/tmp/my-environment.json"
./run-tests.sh
```

## Related Documentation

- **Development Setup:** See `README.md`
- **Test Configurations:** `test/configs/` directory
- **SCION Documentation:** https://docs.scion.org/

## Support

For issues or questions:
1. Check the troubleshooting section above
2. Review logs in `/tmp/forward-proxy.log` and `/tmp/reverse-proxy.log`
3. Verify SCION setup with `cd ~/scion && ./scion.sh status`