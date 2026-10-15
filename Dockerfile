# syntax=docker/dockerfile:1
#
#Multi-stage build: front-end static export → Go single binary (embedded front-end) → running image.
#The original Docker Hub image source (autumn27/artex) has expired. This warehouse now supports self-service construction from pure source code:
#docker compose build # or docker build -t artex:local .
#TARGETARCH is automatically injected by buildx (amd64/arm64), and QEMU is not required for Go cross-compilation;
#The runtime layer only installs tools, and all compilation is completed during the build phase.
# Override registries when a local mirror is required:
#   --build-arg GOPROXY=https://goproxy.cn,direct
#   --build-arg NPM_REGISTRY=https://registry.npmjs.org

########## Phase 1: Front-end static export ##########
FROM node:22.23.3-bookworm AS web
ARG NPM_REGISTRY=https://registry.npmjs.org
WORKDIR /web
#--ignore-scripts: Skip prepare(husky) - without .git in the container, husky will fail;
#The platform binary of Next.js/Biome adopts optionalDependencies and does not rely on life cycle scripts.
COPY web/package.json web/package-lock.json ./
RUN npm config set registry "$NPM_REGISTRY" && npm ci --ignore-scripts
COPY web/ ./
RUN NEXT_EXPORT=1 npm run build:static

########## Phase 2: Go backend (embedded frontend, static compilation) ##########
FROM golang:1.26.3-bookworm AS gobuild
ARG TARGETARCH
ARG GOPROXY=https://proxy.golang.org,direct
WORKDIR /src
COPY go.mod go.sum ./
COPY norma/go.mod norma/go.sum ./norma/
COPY third_party/go-mitmproxy/go.mod third_party/go-mitmproxy/go.sum ./third_party/go-mitmproxy/
RUN go mod download
COPY . .
#The front-end static products are copied from the web stage (.dockerignore has excluded the host dist and will not conflict)
COPY --from=web /web/out/ server/webui/dist/
RUN CGO_ENABLED=0 GOARCH="${TARGETARCH}" go build -tags embedui -trimpath -o /out/artex ./cmd/artex

########## Stage 3: Run Image (same as original: Tools + Playwright pre-installed) ##########
FROM python:3.12-slim-bookworm
ARG NPM_REGISTRY=https://registry.npmjs.org
# Use the official npm registry and default Playwright browser download host.
ENV npm_config_registry=$NPM_REGISTRY
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
#Commonly used tools: ripgrep/curl/vim, plus a batch of recon spare parts (add or delete as needed).
#Node installs 22.x from NodeSource: the apt nodejs that comes with bookworm is 18, and Playwright requires >=20.
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates ripgrep curl wget vim git jq unzip \
      dnsutils iputils-ping netcat-openbsd inetutils-telnet whois nmap \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*
#Playwright MCP and CLI (global) are pre-installed, and no npx network download is required when running.
#@playwright/mcp: browser MCP directly `npx @playwright/mcp` (installed globally, no need for -y/@latest).
#@playwright/cli: Provide playwright-cli. After installation, use --help to verify that it is executable.
#Then install playwright (which provides browser management). After installation, use --with-deps to preset chromium and its system dependencies.
#In this way, the MCP/CLI in the container is available for the first time, and the browser is no longer downloaded from the Internet.
RUN npm install -g @playwright/mcp@0.0.83 @playwright/cli@0.1.22 playwright@1.64.0 \
    && playwright-cli --help \
    && playwright install --with-deps chromium \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
# Chromium's user trust store is configured at runtime for the generated public
# capture CA. Keep this after browser installation so tool updates reuse its layer.
RUN apt-get update && apt-get install -y --no-install-recommends libnss3-tools \
    && rm -rf /var/lib/apt/lists/*
#Single binary produced during the build phase (embedded frontend)
COPY --from=gobuild /out/artex /app/artex
#Guardian startup script: After the process exits, press the exit code to decide whether to restart it. One-click page update relies on it to complete the change.
#It is also responsible for forwarding SIGTERM to artex - docker stop only sends the signal to PID 1,
#If it is not forwarded, artex will not be able to receive or shut down gracefully, and will be hard-killed by SIGKILL after 10 seconds.
COPY start.sh /app/start.sh
# Windows checkouts may use CRLF; the Linux entrypoint needs LF.
RUN sed -i 's/\r$//' /app/start.sh && chmod +x /app/artex /app/start.sh \
    && groupadd --gid 10001 artex && useradd --uid 10001 --gid 10001 --create-home artex \
    && mkdir -p /app/auth /app/data && chown artex:artex /app/auth /app/data \
    && ln -s /app/auth/jwt.key /app/jwt.key
COPY skills/ /app/skills/
# Workspace and signing key persist separately; the key stays outside the browsable workspace.
VOLUME ["/app/data", "/app/auth"]
ENV HOME=/home/artex ARTEX_CA_DIR=/app/auth/ca ARTEX_CHROMIUM_CA_TRUST=1
USER 10001:10001
EXPOSE 8787 8788
ENTRYPOINT ["/app/start.sh"]
CMD ["-addr", ":8787", "-proxy", ":8788"]
