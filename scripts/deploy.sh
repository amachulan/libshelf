#!/usr/bin/env bash
# Deploy libshelf binary built from current master (GitHub pre-release "latest").
# Waits for CI to publish that commit — avoids installing the previous build.
# Does NOT re-import inpx / touch the SQLite DB.
set -euo pipefail

REPO="${LIBSHELF_REPO:-amachulan/libshelf}"
BIN="${LIBSHELF_BIN:-/opt/libshelf/libshelf}"
DATA_DIR="${LIBSHELF_DATA:-/opt/libshelf/data}"
LIB_DIR="${LIBSHELF_LIB:-/mnt/share/Книги/fb2.Flibusta.Net}"
# Optional extra archive roots (colon-separated), e.g. /data/books-new
LIB_DIR_EXTRA="${LIBSHELF_LIB_EXTRA:-}"
ADDR="${LIBSHELF_ADDR:-127.0.0.1:12380}"
SCREEN_NAME="${LIBSHELF_SCREEN:-libshelf}"
ASSET="libshelf-linux-amd64"
# How long to wait for GitHub Actions to publish the master commit.
WAIT_SECS="${LIBSHELF_DEPLOY_WAIT:-600}"

TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

api() {
  local path="$1"
  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    gh api "repos/${REPO}${path}"
    return
  fi
  curl -fsSL \
    -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "https://api.github.com/repos/${REPO}${path}"
}

master_sha() {
  api "/commits/master" | python3 -c 'import json,sys; print(json.load(sys.stdin)["sha"])'
}

release_commit() {
  api "/releases/tags/latest" | python3 -c '
import json,sys,re
body = json.load(sys.stdin).get("body") or ""
m = re.search(r"commit ([0-9a-f]{7,40})", body)
print(m.group(1) if m else "")
'
}

asset_id() {
  api "/releases/tags/latest" | python3 -c '
import json,sys
name = sys.argv[1]
for a in json.load(sys.stdin).get("assets") or []:
    if a.get("name") == name:
        print(a["id"])
        break
' "$ASSET"
}

download_asset() {
  local out="$1"
  local id
  id="$(asset_id)"
  if [[ -z "$id" || "$id" == "null" ]]; then
    echo "Release asset $ASSET not found" >&2
    return 1
  fi

  # Download by asset id — avoids CDN serving a clobbered-but-stale "latest" file.
  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    echo "Downloading asset id=$id via gh ..."
    gh api -H "Accept: application/octet-stream" "/repos/${REPO}/releases/assets/${id}" >"$out"
    return
  fi

  echo "Downloading asset id=$id via curl ..."
  curl -fsSL \
    -H "Accept: application/octet-stream" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    -o "$out" \
    "https://api.github.com/repos/${REPO}/releases/assets/${id}"
}

sha_match() {
  local want="$1"
  local got="$2"
  [[ -n "$got" ]] || return 1
  [[ "$got" == "$want" || "$want" == "$got"* || "$got" == "$want"* ]]
}

wait_for_release() {
  local want="$1"
  local want_short="${want:0:7}"
  local deadline=$((SECONDS + WAIT_SECS))
  echo "Want build for master ${want_short}"
  echo "Waiting up to ${WAIT_SECS}s for GitHub release latest to publish that commit ..."

  while (( SECONDS < deadline )); do
    local got
    got="$(release_commit || true)"
    if sha_match "$want" "$got"; then
      echo "Release is ready (commit ${got})"
      return 0
    fi
    echo "  currently: ${got:-none} — sleeping 10s"
    sleep 10
  done

  echo "Timed out waiting for commit $want in release notes." >&2
  echo "Check Actions: https://github.com/${REPO}/actions" >&2
  exit 1
}

TARGET_SHA="$(master_sha)"
wait_for_release "$TARGET_SHA"

OUT="$TMP/$ASSET"
download_asset "$OUT"
chmod +x "$OUT"

GOT_VER="$("$OUT" version 2>/dev/null || true)"
if [[ "$GOT_VER" != "$TARGET_SHA" ]]; then
  echo "Downloaded binary version mismatch:" >&2
  echo "  want: $TARGET_SHA" >&2
  echo "  got:  ${GOT_VER:-<empty>}" >&2
  echo "Retrying download once after 15s ..." >&2
  sleep 15
  download_asset "$OUT"
  chmod +x "$OUT"
  GOT_VER="$("$OUT" version 2>/dev/null || true)"
  if [[ "$GOT_VER" != "$TARGET_SHA" ]]; then
    echo "Still mismatched after retry — aborting." >&2
    exit 1
  fi
fi
echo "Binary OK: $GOT_VER"

install -d "$(dirname "$BIN")"
install -m 755 "$OUT" "${BIN}.new"

echo "Stopping old process (if any) ..."
if screen -list 2>/dev/null | grep -q "[.]${SCREEN_NAME}[[:space:]]"; then
  screen -S "$SCREEN_NAME" -X quit || true
  sleep 1
fi
pkill -f "${BIN} serve" 2>/dev/null || true
sleep 1

mv -f "${BIN}.new" "$BIN"

echo "Starting $BIN on $ADDR ..."
AUTH_MODE="${LIBSHELF_AUTH:-users}"

SERVE_ARGS=(
  serve
  --addr "$ADDR"
  --data-dir "$DATA_DIR"
  --auth "$AUTH_MODE"
  --library-dir "$LIB_DIR"
)
if [[ -n "$LIB_DIR_EXTRA" ]]; then
  IFS=':' read -r -a _extra_libs <<< "$LIB_DIR_EXTRA"
  for d in "${_extra_libs[@]}"; do
    [[ -n "$d" ]] || continue
    SERVE_ARGS+=(--library-dir "$d")
  done
fi

screen -dmaS "$SCREEN_NAME" "$BIN" "${SERVE_ARGS[@]}"

HEALTH=""
for i in $(seq 1 30); do
  sleep 1
  HEALTH="$(curl -fsS "http://${ADDR}/health" || true)"
  if [[ "$HEALTH" == ok*"$TARGET_SHA"* ]]; then
    echo "OK: http://${ADDR}/health -> ${HEALTH//$'\n'/}"
    exit 0
  fi
done

echo "WARN: health check unexpected: ${HEALTH:-<failed>}" >&2
echo "screen sessions:" >&2
screen -ls >&2 || true
echo "Try foreground:" >&2
echo "  $BIN ${SERVE_ARGS[*]}" >&2
exit 1
