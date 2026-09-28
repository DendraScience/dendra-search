#!/usr/bin/env bash
# Build dendra-search browser bindings for GOOS=js GOARCH=wasm.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

OUT="${1:-examples/wasm/search.wasm}"
GOOS=js GOARCH=wasm go build -o "$OUT" ./examples/wasm

GOROOT="$(go env GOROOT)"
EXEC_DST="$(dirname "$OUT")/wasm_exec.js"
if [[ -f "$GOROOT/lib/wasm/wasm_exec.js" ]]; then
  cp "$GOROOT/lib/wasm/wasm_exec.js" "$EXEC_DST"
elif [[ -f "$GOROOT/misc/wasm/wasm_exec.js" ]]; then
  cp "$GOROOT/misc/wasm/wasm_exec.js" "$EXEC_DST"
else
  echo "wasm_exec.js not found under $GOROOT" >&2
  exit 1
fi

echo "wrote $OUT and $EXEC_DST"
if command -v gzip >/dev/null; then
  gzip -c "$OUT" | wc -c | awk '{printf "gzip size: %.1f MB\n", $1/1024/1024}'
fi
