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

# The server and the browser app show internal/version.Version, and the package
# is versioned in package.yml: one bump, or the settings and the store disagree.
pkg_version="$(sed -n 's/^version: *//p' lazycat/package.yml)"
code_version="$(sed -n 's/^const Version = "\(.*\)"/\1/p' internal/version/version.go)"
if [ "$pkg_version" != "$code_version" ]; then
	echo "internal/version/version.go says $code_version, lazycat/package.yml says $pkg_version" >&2
	exit 1
fi

./scripts/build-web.sh

echo "building the server for linux/amd64"
rm -rf lazycat/content
mkdir -p lazycat/content/web
# The MyGo audio packages pull in purego, whose dlopen shims make an
# ordinary Go binary need the glibc loader; the runtime here is musl-based
# Alpine. Link against static libc instead, with the pure-Go resolver.
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 "$GO" build -tags netgo,osusergo \
	-trimpath -ldflags='-s -w -linkmode external -extldflags "-static"' \
	-o lazycat/content/music-server ./cmd/server
if ! file lazycat/content/music-server | grep -q 'statically linked'; then
	echo "the server binary is not statically linked; it will not run on Alpine" >&2
	exit 1
fi

cp web/index.html web/lzc-media.js web/app.wasm web/app.wasm.gz web/wasm_exec.js lazycat/content/web/
# LRCLib is reached over HTTPS; pack a CA bundle in case the runtime has none.
cp /etc/ssl/certs/ca-certificates.crt lazycat/content/ca-certificates.crt

find lazycat/content -type d -exec chmod 755 {} \;
find lazycat/content -type f -exec chmod 644 {} \;
chmod 755 lazycat/content/music-server

echo "packing"
mkdir -p lazycat/dist
(cd lazycat && $LZC_CLI project build . -f lzc-build.yml)
ls -lh lazycat/dist/*.lpk

# The netdisk the LazyCat client sideloads from gets the new package at once,
# and keeps only it: the older ones go to the netdisk's trash.
NETDISK="${NETDISK:-/lzcapp/document}"
lpk="$(ls -t lazycat/dist/*-v"$pkg_version".lpk | head -1)"
if [ -d "$NETDISK" ]; then
	mkdir -p "$NETDISK/.Trash"
	for old in "$NETDISK"/cloud.lazycat.app.mygomusic-v*.lpk; do
		[ -e "$old" ] || continue
		mv "$old" "$NETDISK/.Trash/"
	done
	cp "$lpk" "$NETDISK/"
	if ! cmp -s "$lpk" "$NETDISK/$(basename "$lpk")"; then
		echo "the copy on the netdisk isn't the one built" >&2
		exit 1
	fi
	echo "delivered $(basename "$lpk") to $NETDISK ($(md5sum "$lpk" | cut -d' ' -f1))"
else
	echo "not delivering: no $NETDISK on this machine"
fi
