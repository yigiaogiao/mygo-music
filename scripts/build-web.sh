#!/usr/bin/env bash
# Builds the browser app: the whole MyGo UI as WebAssembly, and the script
# that starts it. The server serves these from web/.
set -euo pipefail
cd "$(dirname "$0")/.."

GO="${GO:-go}"
mkdir -p web

GOOS=js GOARCH=wasm "$GO" build -trimpath -ldflags='-s -w' -o web/app.wasm .
cp "$("$GO" env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js
gzip -9 -k -f web/app.wasm

echo "built web/app.wasm ($(du -h web/app.wasm | cut -f1)), $(du -h web/app.wasm.gz | cut -f1) gzipped"
