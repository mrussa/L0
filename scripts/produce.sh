#!/usr/bin/env bash
set -euo pipefail

TOPIC="${TOPIC:-orders}"
BROKER_SERVICE="${BROKER_SERVICE:-redpanda}"

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <path-to-json> [key]" >&2
  exit 1
fi

FILE="$1"; shift || true
[[ -f "$FILE" ]] || { echo "No such file: $FILE" >&2; exit 1; }

if [[ $# -ge 1 ]]; then
  KEY="$1"
else
  if command -v jq >/dev/null 2>&1; then
    KEY="$(jq -r '.order_uid // empty' "$FILE")"
  else
    KEY="$(grep -oE '"order_uid"\s*:\s*"[^"]+"' "$FILE" | head -1 | sed -E 's/.*"order_uid"\s*:\s*"([^"]+)".*/\1/')"
  fi
fi

[[ -n "${KEY:-}" ]] || { echo "Cannot determine key (order_uid). Pass it explicitly as 2nd arg." >&2; exit 1; }

echo "Producing to topic '$TOPIC' with key '$KEY' from '$FILE'..."
jq -c . "$FILE" | docker compose exec -T "$BROKER_SERVICE" rpk topic produce "$TOPIC" -k "$KEY"
echo "Done."
