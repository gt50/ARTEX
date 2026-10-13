#!/usr/bin/env bash
# Development mode: run the backend (:8787), traffic proxy (:8788), and frontend dev server (:5173).
# The frontend proxies /api to the backend; Ctrl+C stops all processes.
#
# For a single binary with an embedded frontend, see the production build instructions in README.md.
set -euo pipefail
cd "$(dirname "$0")"

# Stop all child processes (backend and frontend) on exit.
cleanup() { kill 0 2>/dev/null || true; }
trap cleanup EXIT INT TERM

# Backend (plain go run, without an embedded frontend); configure concurrent workers in Settings.
go run ./cmd/artex -addr :8787 -proxy 127.0.0.1:8788 &

# Frontend hot reload (Next dev server; /api proxies to :8787).
( cd web && npm run dev ) &

echo "[dev] backend :8787 / proxy :8788 / frontend http://localhost:5173 (Ctrl+C to stop)"
wait
