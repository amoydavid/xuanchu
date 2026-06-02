#!/usr/bin/env bash
set -euo pipefail

mkdir -p dist
VERSION="${VERSION:-dev}"
for target in \
  linux/amd64 \
  linux/arm64 \
  darwin/amd64 \
  darwin/arm64 \
  windows/amd64
do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"
  suffix=""
  if [ "$GOOS" = "windows" ]; then suffix=".exe"; fi
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -ldflags "-X main.version=${VERSION}" \
      -o "dist/taskg-${VERSION}-${GOOS}-${GOARCH}${suffix}" ./cmd/taskg
done
echo "Built 5 binaries in dist/"
