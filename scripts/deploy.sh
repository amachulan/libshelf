#!/usr/bin/env bash
# Deploy latest libshelf binary from GitHub pre-release "latest".
# Does NOT re-import inpx / touch the SQLite DB.
set -euo pipefail

REPO="${LIBSHELF_REPO:-amachulan/libshelf}"
BIN="${LIBSHELF_BIN:-/opt/libshelf/libshelf}"
DATA_DIR="${LIBSHELF_DATA:-/opt/libshelf/data}"
LIB_DIR="${LIBSHELF_LIB:-/mnt/share/Книги/fb2.Flibusta.Net}"
ADDR="${LIBSHELF_ADDR:-127.0.0.1:12380}"
SCREEN_NAME="${LIBSHELF_SCREEN:-libshelf}"
ASSET="libshelf-linux-amd64"

TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

download_latest() {
  local out="$1"
  local url="https://github.com/${REPO}/releases/download/latest/${ASSET}"

  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    echo "Downloading via gh ($REPO @ latest) ..."
    gh release download latest -R "$REPO" -p "$ASSET" -D "$TMP"
    mv "$TMP/$ASSET" "$out"
    return
  fi

  if command -v curl >/dev/null 2>&1; then
    echo "Downloading via curl: $url"
    curl -fL --retry 3 -o "$out" "$url"
    return
  fi

  if command -v wget >/dev/null 2>&1; then
    echo "Downloading via wget: $url"
    wget -O "$out" "$url"
    return
  fi

  echo "Need curl, wget, or authenticated gh CLI" >&2
  exit 1
}

download_latest "$TMP/$ASSET"
chmod +x "$TMP/$ASSET"

install -d "$(dirname "$BIN")"
install -m 755 "$TMP/$ASSET" "${BIN}.new"

echo "Stopping old process (if any) ..."
if screen -list 2>/dev/null | grep -q "[.]${SCREEN_NAME}[[:space:]]"; then
  screen -S "$SCREEN_NAME" -X quit || true
  sleep 1
fi
pkill -f "${BIN} serve" 2>/dev/null || true
sleep 1

mv -f "${BIN}.new" "$BIN"

echo "Starting $BIN on $ADDR ..."
# --auth=users is the default; set LIBSHELF_AUTH=none to disable login.
AUTH_MODE="${LIBSHELF_AUTH:-users}"

screen -dmaS "$SCREEN_NAME" "$BIN" serve \
  --addr "$ADDR" \
  --library-dir "$LIB_DIR" \
  --data-dir "$DATA_DIR" \
  --auth "$AUTH_MODE"

sleep 1
if curl -fsS "http://${ADDR}/health" >/dev/null; then
  echo "OK: http://${ADDR}/health"
else
  echo "WARN: health check failed; check: screen -r $SCREEN_NAME" >&2
  exit 1
fi
