#!/usr/bin/env bash
# Builds the LazyCat package without Docker: the server binary and the
# WebAssembly UI go into lazycat/content, which lzc-cli turns into content.tar.
set -euo pipefail
cd "$(dirname "$0")/.."

GO="${GO:-go}"
LZC_CLI="${LZC_CLI:-}"
if [ -z "$LZC_CLI" ]; then
	if command -v lzc-cli >/dev/null 2>&1; then
		LZC_CLI="$(command -v lzc-cli)"
	elif [ -x "$HOME/.local/share/lzctools/node_modules/.bin/lzc-cli" ]; then
		LZC_CLI="$HOME/.local/share/lzctools/node_modules/.bin/lzc-cli"
	else
		LZC_CLI="npx --yes @lazycatcloud/lzc-cli@2.0.9"
	fi
fi

./scripts/build-web.sh

echo "building the server for linux/amd64"
rm -rf lazycat/content
mkdir -p lazycat/content/web
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -trimpath -ldflags='-s -w' \
	-o lazycat/content/music-server ./cmd/server

cp web/index.html web/app.wasm web/app.wasm.gz web/wasm_exec.js lazycat/content/web/
# LRCLib is reached over HTTPS; pack a CA bundle in case the runtime has none.
cp /etc/ssl/certs/ca-certificates.crt lazycat/content/ca-certificates.crt

find lazycat/content -type d -exec chmod 755 {} \;
find lazycat/content -type f -exec chmod 644 {} \;
chmod 755 lazycat/content/music-server

echo "packing"
mkdir -p lazycat/dist
(cd lazycat && $LZC_CLI project build . -f lzc-build.yml)
ls -lh lazycat/dist/*.lpk
