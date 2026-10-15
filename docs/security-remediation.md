# October 2026 penetration test remediation

## Initial patch: finding 1 (MCP subprocess execution)

Stdio MCP launches are now denied by default. The operator must supply
`ARTEX_MCP_STDIO_ALLOWLIST` in the server process environment as a JSON array of
exact launch recipes. Each recipe has `command` (an absolute executable path),
`args` (an ordered string array), and `env` (a string map). Example:

```json
[{"command":"/usr/bin/npx","args":["@playwright/mcp","--headless"],"env":{}}]
```

Only approve trusted executables and code. An approved interpreter or package
runner still executes code with the console's privileges. Keep its executable,
packages, working directory, and dependencies protected against modification.
Do not approve shell launch recipes or attacker-writable scripts. Policy is
deployment configuration, not an API/database/workspace file. Restart the server
after changing deployment configuration.

Registration and agent create/update tools reject unapproved recipes. The shared
connection path checks again before spawning, covering existing database rows,
startup discovery, refresh, and agent tool assembly. HTTP/SSE transports retain
their current behavior. Browser capture synchronization changes proxy arguments
and CA environment variables: explicitly approve the exact resulting recipes for
each capture state you use. There is no automatic exception for the seeded browser.

The bundled Linux browser needs these two explicit recipes:

```json
[{"command":"/usr/bin/npx","args":["@playwright/mcp","--headless"],"env":{}},{"command":"/usr/bin/npx","args":["@playwright/mcp","--headless","--config","/app/auth/ca/browser-mcp.json"],"env":{"NODE_EXTRA_CA_CERTS":"/app/auth/ca/mitmproxy-ca-cert.pem"}}]
```

The local deployment configuration contains these recipes. Its browser row was
changed to `/usr/bin/npx` after matching the seeded arguments and environment.
The approved server discovered 25 tools, and Chromium successfully forwarded
through the generated authenticated proxy configuration.

HTTPS capture exposed a separate browser-trust gap: Node's CA environment does
not configure Chromium's certificate store. The Docker image now installs NSS
tools and opts into importing only the public capture CA into the service user's
ephemeral home. Native installs require explicit `ARTEX_CHROMIUM_CA_TRUST=1` and
Linux; Windows certificate stores are never changed by this helper. The helper
uses [Chromium's documented NSS store](https://chromium.googlesource.com/chromium/src/+/main/docs/linux/cert_management.md),
replaces only its own named certificate, and removes it when capture is disabled.
Live checks passed for authenticated HTTPS capture, rejection of a direct untrusted
certificate, and trust removal on capture-off. TLS verification remains enabled.

## Session and browser changes

Finding 9: the API now accepts bearer credentials only from Authorization. All
four stream consumers use a fetch-based SSE client with header authentication,
event IDs, named events, and reconnection. URLs carry no credential. The client
clears the legacy cookie instead of mirroring tokens into it; Next development
routing relies on the existing client authentication guard. Token storage remains
in localStorage and should be considered in any future session redesign.

Finding 10: login admission is limited to 10 attempts per source IP and 60 total
per minute, including malformed and concurrent requests, with 429/Retry-After.
Forwarding headers do not control identity. Behind a reverse proxy, callers share
its source IP limit. This is an in-process limit, not a multi-instance quota.

Finding 11: all backend responses receive frame, MIME, referrer, permissions, and
CSP headers; API responses disable caching. HSTS is emitted on actual TLS
connections. Embedded HTML adds SHA-256 hashes for its exact inline hydration
scripts; arbitrary inline scripts are denied. The static frontend build passes.

The full Go suite passes against a disposable PostgreSQL database after updating
the management fixture to assert stdio rejection and stopping background task
loops before test-directory cleanup. The regression cases pass three consecutive
runs. Frontend TypeScript, static export, and the hardened container build pass.
The SSE parser/reconnection tests pass with split UTF-8, CRLF, named events,
multiline data, Last-Event-ID, and authorization failures. The candidate login
page hydrates without CSP console errors.

A separate non-production candidate container passed live checks for header-only
sessions, denied shell registration, HTTP and CONNECT proxy authentication,
authorized forwarding, capture-header redaction in SQLite, login 429/Retry-After,
CA permissions (0700 directory, 0600 PEM), UID 10001, and read-only binary/skills.
Structured-body and stored-capture redaction are included in the rebuilt image.
A further containment audit replaced workspace and chat-upload filesystem
operations with `os.Root`: leaf/ancestor symlinks cannot reach files outside the
workspace. Downloads use a confined open file descriptor; chat uploads create
unique files atomically. Normal operations and symlink escapes have regression
tests. The fresh full Linux suite passed after this patch. Live candidate checks
denied symlink reads of the CA/signing key, downloads, listing, writes and deletes;
the private sentinel remained unchanged. The same image passed proxy, session,
rate-limit and authenticated SSE checks.

Windows endpoint protection blocked a generated database test executable. No
exclusions or protection changes were made. The full suite subsequently passed
in the Linux build container using a fresh disposable database. Reproduce with
`./scripts/test-security.ps1 -Linux`. Windows symlink tests require developer
privileges; release checks exercise them in Linux.

## Proxy, CA and container changes

The recording proxy requires Basic authentication before forwarding HTTP or
CONNECT. `ARTEX_PROXY_TOKEN` supplies a stable password (minimum 32 bytes); when
absent, startup generates a random 256-bit password. Agent proxy URLs include it.
Proxy authorization is stripped before forwarding and capture, and credential URLs
are no longer printed in the startup/browser logs. Browser capture receives
structured Playwright proxy settings through a private configuration file rather
than unsupported credentials in Chromium's proxy-server argument.

Common credential headers, URL query fields, JSON fields (recursively), and form
fields are redacted before new capture, blob and index writes. Live forwarded
requests are not modified. Tests verify that password and response-token values
are absent from the full-text index. Arbitrary unstructured body/path secrets
cannot be identified reliably by field-based redaction; stored captures remain
sensitive. The local legacy store was processed as described below.

CA material now lives outside the workspace under `ARTEX_CA_DIR`, or a per-store
directory in the user's configuration directory. Directories are restricted to
0700 and the private PEM to 0600 on POSIX. Docker sets `/app/auth/ca`. A CA under
the workspace is rejected. Existing workspace CA files are not silently migrated
or deleted: the new location generates a fresh CA, and existing trusted copies
must be removed/replaced during local migration.

The candidate Docker runtime runs as UID/GID 10001. Compose removes writable
repository binds, uses a named workspace volume, drops all capabilities, forbids
privilege escalation, and makes the root filesystem read-only. A separate offline
storage initializer owns the named volumes for the non-root user. Home and tmp
use bounded tmpfs. Bundled skills and the application binary are read-only;
container skill editing and in-place self-update therefore require rebuilding the
image. Existing data must be copied to the named volume before cutover.

## Local maintenance state

The original ARTEX service was stopped for maintenance before migration.
PostgreSQL and unrelated applications remained available. Before changing
data, an authenticated AES-256-GCM recovery archive was created outside the
workspace in the operator's local ARTEX security-recovery directory. Its ACL
allows only that Windows user and SYSTEM. Decrypt-and-compare verification passed.

The eight inline captures were processed with `cmd/redact-traffic`. The tool
refuses legacy file/blob formats before mutation, rebuilds FTS, compacts SQLite,
and truncates WAL. All eight records were preserved; these particular records
contained no credential fields requiring replacement. The old CA fingerprint
matched the report exactly, and its three workspace files were removed after
encrypted backup. No matching mitmproxy certificate was found in Windows root
trust stores. The signing key was rotated, invalidating prior sessions. Four
verified ARTEX_I224 probe skill directories were archived and removed. Reported
MCP rows 12/13 were already absent; legitimate ScopeSentry and browser rows remained.

The cleaned workspace was copied to `artex_artexdata`. Its SHA-256 file/link
manifest matched all 785 entries, ownership was assigned to UID/GID 10001, and an
offline check confirmed eight captures and no workspace CA PEM. The source was
retained. Runtime cutover uses the tested security-only image, with named volumes
and no repository bind mounts. The service is healthy at http://127.0.0.1:8791.

Compose's storage initializer also completed successfully on the migrated volumes.
A separate `codex/security-hardening` worktree contains only the security patch;
the account-management edits present before this task remain in the original
checkout. The final scoped patch passed the complete Linux Go suite, a fresh
container/static build, TypeScript checks, and three SSE regression tests. Live
checks against that final image passed authenticated HTTP/CONNECT forwarding,
header-only sessions, login throttling, filesystem containment, authenticated
SSE, and HTTPS capture with certificate verification enabled. The SSE tests cover
incomplete-frame cursor handling, ID-only resets, and terminal CR delimiters.

Finding 3: after explicit operator approval, `scripts/secure-local-strata.ps1`
validated process paths, set a random key in protected local configuration,
updated only the matching ARTEX profile, and restarted the validated Strata server
and engine. Strata remains bound to 127.0.0.1:8080. Unauthenticated reads of
/config, /settings, /mcp, /metrics, request telemetry, and /v1/models now return
401. Unauthenticated POSTs to /config and /settings also return 401. Authorized
reads and a model completion succeeded; the protected configuration and saved
ARTEX profile contain the same key. Health remains intentionally public.

## Production verification

The initial security cutover used image
`sha256:298c96ea04973a65760c758dcda132ba07bd967ec7ce058f6cb0040bd99daffe`.
Post-cutover checks confirmed healthy HTTP service, UID/GID 10001, a read-only
root filesystem, all capabilities dropped, no-new-privileges, named volumes only,
private signing-key/CA permissions, header-only sessions, authenticated SSE,
25 approved browser MCP tools, and rejection of anonymous HTTP/CONNECT proxy use.
ARTEX successfully listed Strata models using its saved authenticated profile.
The restored task resumed processing its existing 84,399-token prompt; a separate
30-second connection-test completion timed out behind that work. The direct
authorized model-completion check passed before cutover. No task was interrupted
to force the connection test to pass. Both reported i174 marker files are absent.

The reviewed security changes are published on `codex/security-hardening`.
The initial security deployment excluded existing account-management edits and
therefore regressed login to the fixed ARTEX username. A follow-up restores the
custom-account backend, editable login/setup fields, account settings, and UTF-8
token display while preserving header-only authentication and login throttling.
The saved production credentials are preserved. A disposable deployment verified
custom-username login, rejection of ARTEX for a custom account, authenticated
account lookup, and an editable hydrated login field in Chromium.
The follow-up runtime image is
`sha256:7ebb5d5323714dc5c9726a1e27c20ebdf738f734ea0dad1df81242ddab78d1a7`.

## Remaining proxy hardening

The follow-up removes unconditional upstream TLS bypass. Certificates are
verified against the actual CONNECT destination, rather than client-supplied SNI.
The interception certificate also names that destination, including IP SANs.
There is no automatic fallback to an opaque tunnel after protocol/TLS failures.
`ARTEX_PROXY_CA_FILE` can add a private target CA to system roots; this bundle
must resolve outside the writable workspace. No insecure TLS switch is provided.

`ARTEX_PROXY_ALLOWLIST` is an operator-controlled JSON array of exact host:port
pairs. Blank or empty denies all destinations. Hostnames resolve once at startup,
and the selected IP is pinned for both direct connections and remote proxy
tunnels. The policy is checked before HTTP/CONNECT dialing and on decrypted
HTTP requests. Rewritten requests cannot use the dependency's separate-client
path to bypass that policy. Ports, IP literals, and hostname aliases each require
explicit approval; wildcards and CIDR ranges are not accepted. DNS/scope changes
require restart. The local deployment permits only `127.0.0.1:8787` and
`host.docker.internal:8791`; other pentest targets require an operator update.
This controls the recording proxy, not tools connecting directly without capture.

The runtime portion of go-mitmproxy v1.9.2 is retained under `third_party` with its
license and upstream tests, plus a short patch inventory. This provides the
missing resolver hook before direct and upstream-proxy dials, target CA trust,
destination-bound TLS names, and credential-URL removal from active error logs.

CA location validation now resolves symlinks before accepting the path. Store
directories use 0700 and new SQLite/blob files use 0600 on POSIX. The CA remains
outside the workspace with a private 0600 PEM. Captures still contain intentional
target evidence after recognized credential fields are redacted. They are not
encrypted at rest, and the service must be able to read its signing keys and
capture data. These are sensitive service-owned resources, not secrets protected
from arbitrary execution as that same service user. The recovery archive remains
encrypted outside the workspace.

Regression coverage includes denied destinations receiving no TCP connection,
approved HTTP and HTTPS, repeated rejection of untrusted certificates, explicit
private-CA trust, SNI/hostname mismatch rejection, pinned destinations through an
upstream proxy, empty/invalid policies, and private-file/CA path permissions.

The final Linux application suite passed, as did the dependency's local
connection, WebSocket, SSE, certificate/helper, and credential-log regressions.
The hardened container build passed. A disposable runtime confirmed anonymous
407 responses; authenticated 403 responses for unlisted Strata, PostgreSQL,
metadata and loopback destinations; approved HTTP forwarding; trusted HTTPS;
three consecutive untrusted-certificate rejections; verified Chromium capture;
custom-username login; header-only SSE; and approved browser MCP discovery.

The deployed image is
`sha256:5ce7efd7893f4c30fdef1801208b83bd8ff2d364cfc68291968942b2b067092f`.
The running environment contains the ARTEX-only allowlist above. Production
checks passed for service health, private file modes, hardened container settings,
named volumes, editable hydrated username field, header-only sessions, streams,
approved MCP discovery, and authenticated Strata model listing. Stored account
credentials were unchanged, and the operator's foreground Strata listener was
not restarted; its anonymous metrics request still returned 401.
