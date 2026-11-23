#!/usr/bin/env bash
# Build kardex-pdf-service and publish it to the Fe-Go-Prod repo (the one the "nuevo" server pulls).
#
# Usage:  scripts/release-prod.sh [-m "message"] [-y]
#   -m  commit message suffix (default: last Go-Service commit subject)
#   -y  skip the push confirmation
#
# Env:    GO_PROD_DIR  local clone of Fe-Go-Prod (default: <codicore>/builds/go/Fe-Go-Prod)
#
# This script never touches the server. Deploy by hand afterwards:
#   ssh nuevo 'cd /var/www/fe-go && git pull && pm2 restart go-pdf-worker'
set -euo pipefail

SERVICE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_PROD_REPO="git@github.com:CodicoreSAC/Fe-Go-Prod.git"
GO_PROD_DIR="${GO_PROD_DIR:-$(cd "$SERVICE_DIR/../.." && pwd)/builds/go/Fe-Go-Prod}"
BINARY_REL="fe/kardex-pdf-service"

MESSAGE=""
ASSUME_YES=0
while getopts "m:y" opt; do
  case "$opt" in
    m) MESSAGE="$OPTARG" ;;
    y) ASSUME_YES=1 ;;
    *) echo "Usage: $0 [-m \"message\"] [-y]" >&2; exit 2 ;;
  esac
done

step() { printf '\n==> %s\n' "$1"; }
die() { printf 'ERROR: %s\n' "$1" >&2; exit 1; }

command -v go >/dev/null || die "go is not installed"

# 1) Preflight on the source repo (Go-Service)
step "Preflight: Go-Service must be clean and pushed"
cd "$SERVICE_DIR"
[ -z "$(git status --porcelain --untracked-files=no -- .)" ] || die "uncommitted changes in go-service; commit them first"
git fetch -q
UNPUSHED="$(git rev-list --count '@{u}..HEAD' 2>/dev/null || echo 0)"
[ "$UNPUSHED" = "0" ] || die "go-service has $UNPUSHED unpushed commit(s); push first so prod matches GitHub"
SRC_HASH="$(git rev-parse --short HEAD)"
[ -n "$MESSAGE" ] || MESSAGE="$(git log -1 --format=%s)"

step "go vet + go test"
go vet ./...
go test ./...

# 2) Fe-Go-Prod clone
step "Fe-Go-Prod clone at $GO_PROD_DIR"
if [ ! -d "$GO_PROD_DIR/.git" ]; then
  mkdir -p "$(dirname "$GO_PROD_DIR")"
  git clone "$GO_PROD_REPO" "$GO_PROD_DIR"
fi
cd "$GO_PROD_DIR"
git checkout -q main
[ -z "$(git status --porcelain --untracked-files=no)" ] || die "Fe-Go-Prod clone has uncommitted changes"
git pull --rebase -q

# 3) Build (static linux/amd64: the code has no real cgo)
step "Build $BINARY_REL (go-service @ $SRC_HASH)"
mkdir -p "$(dirname "$BINARY_REL")"
( cd "$SERVICE_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" -o "$GO_PROD_DIR/$BINARY_REL" ./cmd/server )
chmod +x "$BINARY_REL"
SIZE="$(du -h "$BINARY_REL" | cut -f1)"
echo "built $BINARY_REL ($SIZE)"

# 4) Commit
if [ -z "$(git status --porcelain -- "$BINARY_REL")" ]; then
  echo "Binary identical to the one in Fe-Go-Prod; nothing to release."
  exit 0
fi
git add -- "$BINARY_REL"
git commit -q -m "release: kardex-pdf-service $SRC_HASH: $MESSAGE"
echo "committed: $(git log -1 --format='%h %s')"

# 5) Push
if [ "$ASSUME_YES" != "1" ]; then
  read -r -p "Push to origin/main of Fe-Go-Prod? [y/N] " ans
  [ "$ans" = "y" ] || { echo "Not pushed. Commit stays local in $GO_PROD_DIR"; exit 0; }
fi
git push -q origin main
echo "pushed: $(git rev-parse --short HEAD)"

# 6) Next step (manual)
step "Done. Deploy on the server when ready:"
echo "  ssh nuevo 'cd /var/www/fe-go && git pull && pm2 restart go-pdf-worker'"
