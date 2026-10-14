# Local Docker deployment

This stack follows the local deployment conventions used by RentLens and Dividend Orchard: build the application from this checkout, publish ports only on localhost, persist data, wait for PostgreSQL to become healthy, and restart when Docker starts.

## Start

1. Copy `.env.example` to `.env` and replace `POSTGRES_PASSWORD` with a long random password. Use letters and digits (or hexadecimal) so the password can be embedded in the database connection URL without escaping. The local deployment already has a generated password in the ignored `.env` file.
2. Run `docker compose up -d --build` from this repository.
3. Open [ARTEX](http://127.0.0.1:8791) and complete the first-run setup. Configure an LLM provider in the interface when you want to use agent features.

The image is built locally as `artex:local`; it contains the English interface, the restored Norma source dependency, reconnaissance tools, and Chromium with Playwright. It does not pull the older upstream ARTEX application image.

## Ports

| Service | Local address | Setting |
| --- | --- | --- |
| ARTEX UI and API | `127.0.0.1:8791` | `ARTEX_PORT` |
| Capture proxy | `127.0.0.1:8788` | `ARTEX_PROXY_PORT` |
| PostgreSQL | `127.0.0.1:5434` | `ARTEX_DB_PORT` |

RentLens keeps its existing ports 3100/5432; Dividend Orchard keeps 8790/5433. Containers reach the ARTEX database at `postgres:5432`.

For a model server running on this computer, set `DOCKER_LLM_BASE_URL` to its URL using `host.docker.internal`, for example `http://host.docker.internal:1234/v1`. Inside a container, localhost refers to that container. API keys and provider/model settings can also be configured using the existing `.env` variables.

## Persistent data

- PostgreSQL records live in the Compose-managed `pgdata` volume.
- ARTEX files, evidence, and SQLite traffic records live in the Compose-managed `artexdata` volume at `/app/data`.
- The JWT signing key and private interception CA live in `authdata` at `/app/auth`, outside the browsable workspace.
- Bundled skills are copied into the image. The runtime filesystem, application binary, and skills are read-only; update them by rebuilding the image.
- `.env` holds local secrets and is excluded from Git and the Docker build context.

The app runs as UID/GID 10001 with all capabilities dropped and privilege escalation disabled. An offline one-shot initializer assigns the named volumes to that user. Temporary files and the home directory use bounded tmpfs.

Existing installations using `./data` must stop the app and make an encrypted recovery backup before upgrading. Copy the old directory into an empty `artex_artexdata` volume with a trusted offline container, preserve symlinks without following them, assign UID/GID 10001, and compare file/link manifests before cutover. Retain the original until the copy is verified. The October 2026 local migration is recorded in [the remediation report](docs/security-remediation.md).

Stop the app before backing up the workspace volume; use `pg_dump` for database backups. `docker compose down` preserves named volumes. Adding `--volumes` deletes records and keys, so omit that flag when keeping data.

The proxy requires authentication for HTTP and CONNECT. Set `ARTEX_PROXY_TOKEN` to a random password of at least 32 bytes for external capture clients (username `artex`); blank generates a fresh password at startup. ARTEX supplies credentials to its own clients automatically. Stdio MCP launches require an exact `ARTEX_MCP_STDIO_ALLOWLIST` recipe, including an absolute executable path. See the remediation report for browser examples. The public interception certificate is `/app/auth/ca/mitmproxy-ca-cert.pem`; keep the adjacent private PEM private.

The image configures Chromium to trust that public CA in its ephemeral service-user NSS store while capture is enabled, then removes that trust when capture is disabled. Browser TLS verification remains enabled. Native installations do not alter browser trust unless explicitly opted in with `ARTEX_CHROMIUM_CA_TRUST=1` on Linux.

## Daily commands

```bash
docker compose ps
docker compose logs --tail=100 artex
docker compose up -d --build
docker compose restart artex
docker compose stop
docker compose start
```

The app and database use `restart: unless-stopped`. They resume when Docker Desktop starts unless deliberately stopped. The storage initializer exits after setup. This stack is local only; no Tailscale or public deployment is enabled.
