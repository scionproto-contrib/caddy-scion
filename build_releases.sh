#!/bin/bash

# This script builds the release version of the project.

export CGO_ENABLED=0

BUILD_DIR="./build"
mkdir -p "$BUILD_DIR"

PLATFORMS=(
  "linux:amd64:x86_64"
  "linux:arm64:arm64"
  "darwin:amd64:x86_64"
  "darwin:arm64:arm64"
  "windows:amd64:x86_64"
)

BINARIES=(
  "scion-caddy-forward"
  "scion-caddy-reverse"
  "scion-caddy-native"
)

for binary in "${BINARIES[@]}"; do
  for platform in "${PLATFORMS[@]}"; do
    IFS=":" read -r OS ARCH ARCH_NAME <<< "$platform"
    
    if [[ "$OS" == "darwin" && ("$binary" == "scion-caddy-reverse" || "$binary" == "scion-caddy-native") ]]; then
      continue
    fi
    
    if [[ "$OS" == "windows" ]]; then
      OUTPUT="${BUILD_DIR}/${binary}_${ARCH_NAME}.exe"
    else
      OUTPUT="${BUILD_DIR}/${binary}_${OS}_${ARCH_NAME}"
    fi
    
    echo "Building ${binary} for ${OS}/${ARCH}..."
    GOOS=$OS GOARCH=$ARCH go build -o "$OUTPUT" "./cmd/$binary"
  done
done

echo "All builds completed successfully!"