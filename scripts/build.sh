#!/usr/bin/env bash
# 编译：windows + linux 双平台
set -e

OUT_DIR="dist"
mkdir -p $OUT_DIR

# Windows
echo "==> build windows/amd64"
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o $OUT_DIR/faka-gateway.exe .

# Linux
echo "==> build linux/amd64"
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o $OUT_DIR/faka-gateway-linux .

# macOS
echo "==> build darwin/amd64"
GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o $OUT_DIR/faka-gateway-darwin .

ls -la $OUT_DIR
echo "done."
