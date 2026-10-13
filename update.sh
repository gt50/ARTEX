#!/usr/bin/env bash
# ARTEX update script: 1) Docker update (pull a new image and recreate)  2) local rebuild.
# This complements install.sh: install performs the initial setup; update upgrades the release.
# No manual DB migration is needed. On every startup, artex idempotently reapplies schema.sql
# (including ADD COLUMN / CREATE INDEX IF NOT EXISTS), so restarting applies migrations. Data in
# the pgdata volume, ./data, and ./skills is preserved.
set -euo pipefail
cd "$(cd "$(dirname "$0")" && pwd)"

info(){ printf '\033[36m[*]\033[0m %s\n' "$*"; }
ok(){   printf '\033[32m[+]\033[0m %s\n' "$*"; }
warn(){ printf '\033[33m[!]\033[0m %s\n' "$*"; }
die(){  printf '\033[31m[x]\033[0m %s\n' "$*" >&2; exit 1; }
ask(){  local p="$1" d="${2:-}" a; read -rp "$p${d:+ [$d]}: " a; echo "${a:-$d}"; }

# ── Optional: update the checkout (Compose, scripts, and local source builds) ──
sync_repo(){
  [ -d .git ] && command -v git >/dev/null 2>&1 || { warn "Not a Git working copy; skipping git pull"; return; }
  [ "$(ask 'Pull the latest code (git pull --ff-only)? (y/n)' y)" = y ] || return
  if ! git pull --ff-only; then
    warn "git pull could not fast-forward (local changes or diverged branch); resolve this manually and try again. Continuing with the current checkout."
  fi
}

# ── 1) Docker update ─────────────────────────────
update_docker(){
  command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1 \
    || die "docker / docker compose not found; run ./install.sh to set up ARTEX first"
  [ -f .env ] || die ".env not found; run ./install.sh to complete the initial setup"

  # Optionally upgrade to a specific image tag (blank keeps ARTEX_TAG from .env, defaulting to latest).
  local tag; tag="$(ask 'Target image tag (press Enter to keep .env / latest)' '')"
  if [ -n "$tag" ]; then
    if grep -q '^ARTEX_TAG=' .env; then
      sed -i.bak "s|^ARTEX_TAG=.*|ARTEX_TAG=${tag}|" .env && rm -f .env.bak
    else
      printf '\nARTEX_TAG=%s\n' "$tag" >> .env
    fi
    ok "Set ARTEX_TAG to ${tag}"
  fi

  # Update only artex. Postgres uses a fixed 16-alpine image and does not need an update;
  # pulling it wastes bandwidth and major-version changes can cause compatibility issues.
  # artex depends on postgres, so Compose starts it if needed and leaves a running DB untouched.
  info "Pulling the new image (artex only)..."
  docker compose pull artex
  info "Recreating and starting (schema migrations run automatically on startup)..."
  docker compose up -d artex
  ok "Update complete -> http://localhost:8787"
  info "View logs: docker compose logs -f artex"
  info "Optional: remove unused images with docker image prune -f"
}

# ── 2) Local rebuild ─────────────────────────────
update_local(){
  command -v go >/dev/null 2>&1 || die "Go (>=1.26) not found: https://go.dev/dl/"
  [ -f config.json ] || warn "config.json not found; use ./install.sh for the initial setup"
  ok "Go: $(go version)"

  if command -v npm >/dev/null 2>&1; then
    info "Rebuilding static frontend assets..."
    ( cd web && npm ci && npm run build:static )
    rm -rf server/webui/dist && cp -r web/out server/webui/dist
    info "Rebuilding the single binary with embedded frontend..."
    CGO_ENABLED=0 go build -tags embedui -trimpath -o artex ./cmd/artex
  else
    warn "npm not found: building the backend without an embedded frontend (run npm run dev separately for the frontend)"
    CGO_ENABLED=0 go build -o artex ./cmd/artex
  fi
  ok "Build complete -> ./artex"
  warn "Restart the running artex process to apply the update (schema migrations run automatically on startup)"
}

echo "=============================="
echo "  ARTEX Update"
echo "  1) Docker update (pull a new image and recreate)"
echo "  2) Local update (rebuild with Go)"
echo "=============================="
case "$(ask 'Select an option' 1)" in
  1) sync_repo; update_docker ;;
  2) sync_repo; update_local ;;
  *) die "Invalid selection" ;;
esac
