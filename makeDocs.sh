#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

# Go build cache가 리포에 생기지 않도록 임시 디렉토리로 우회합니다.
if GOCACHE_DIR="$(mktemp -d 2>/dev/null)"; then
  :
else
  GOCACHE_DIR="$(mktemp -d -t gocache)"
fi
export GOCACHE="$GOCACHE_DIR"
trap 'rm -rf "$GOCACHE_DIR"' EXIT

echo "$PATH"
SWAG="$ROOT/.bin/swag"
if [[ -x "$SWAG" ]]; then
  echo "using local swag: $SWAG"
else
  SWAG="$(command -v swag)"
fi
echo "$SWAG"
echo "$(pwd)"

rm -f "$ROOT/docs/docs.go" "$ROOT/docs/swagger.json" "$ROOT/docs/swagger.yaml"

echo swag init
"$SWAG" init -g cmd/apiserver/main.go -o docs --parseDependency --parseInternal
