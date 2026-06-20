#!/usr/bin/env bash
# 启动服务（前台）
set -e
cd "$(dirname "$0")/.."
exec ./faka-gateway.exe
