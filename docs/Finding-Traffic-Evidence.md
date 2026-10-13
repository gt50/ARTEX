# Traffic Evidence for Findings

Finding details support selecting multiple traffic records across pages, assigning each a role and note, reordering them, and unbinding them. On the traffic page, select multiple records and attach them to one existing finding. Evidence inherited from a task is read-only; edit it from the task where the finding originated.

The **Agent automatic traffic binding** setting is off by default. It is stored as `agent_traffic_binding` in `/api/settings`. Enabling it may increase token use because the reporting workflow can make additional query requests, tool calls, and prompt input. The next Agent run uses the updated setting. When disabled, automatic-binding parameters and tools are hidden, binding instructions are omitted, and new automatic-binding submissions from running sessions are rejected. Manual binding, traffic capture, reading saved evidence, and exports remain available.

When enabled, the default workflow is: **discover the finding → start the reporting Agent → verify and bind traffic → write the report against the latest evidence version**. The reporter preserves the verification command, key output, real traffic IDs, and their purposes in `evidence`. The reporting Agent reviews the finding and execution record, verifies candidate records with `traffic_search` and `traffic_get`, calls `bind_finding_traffic`, then reads the latest `version` before saving the report. With automatic binding disabled, the reporting Agent receives no additional tools for searching or reading raw traffic and does not bind records automatically; it can still read manually attached snapshots and write reports.

Existing callers remain compatible: `report_finding` can still bind records explicitly using `traffic_refs` or `evidence_hint_id`. Binding is optional. If traffic was not captured (for example, for a TCP finding) or the exact record cannot be found, the finding can still be reported without a traffic reference; retain command output, logs, and other verifiable evidence. Explain why a reference is omitted, and do not add required fields. Every supplied ID and note must be valid and complete. If any binding fails, the binding transaction is rolled back. If `report_finding` reports an explicit binding failure, the entire report is rolled back. Reattaching the same snapshot does not create a duplicate binding or replace its note.

## Finding IDs and report versions

`report_finding` accepts an optional array of traffic references. Array order determines the initial evidence order:

```json
{
  "traffic_refs": [
    {"traffic_id": "real traffic ID", "role": "baseline", "note": "Normal account request"},
    {"traffic_id": "another real traffic ID", "role": "proof", "note": "Reproduces the issue"}
  ]
}
```

Roles are `baseline` (normal control), `proof` (demonstrates the vulnerability), `verification` (additional verification), and `supporting` (supplementary evidence; the default). Use `traffic_search` and `traffic_get` to verify a real record first. Hostnames and timestamps are only for finding candidates; they do not establish task ownership.

The prompt guidance follows [CyberStrikeAI's vulnerability-reporting tools](https://github.com/RuoJi6/CyberStrikeAI/blob/54d56774b8bd285817d16d48b70a4a5e6e0963f7/internal/app/vulnerability_tools.go). ARTEX makes binding optional, so a report does not require an explanation when no traffic record is available. Do not guess IDs or repeat probing just to obtain a packet capture.

The first response line remains `finding recorded: <exploration node ID>`. The following JSON includes the independent vulnerability record ID `finding_id`, exploration-node ID `finding_node_id`, and a binding summary.

- `get_finding_traffic(finding_id)` takes the **independent finding record ID** and returns the ordered evidence list and its `version`. Read a binding's `request` or `response` in chunks using `binding_id`, `side`, `offset`, and `length`; each chunk can be up to 8,192 bytes.
- `update_finding_report` continues to take the exploration-node ID as `finding_id`. Include `evidence_version` with the version actually read. If evidence changes while the report is being generated, the stale version is rejected and the evidence must be read again before regenerating the report.
- Reports created without an evidence version do not claim to cover attached traffic. Changes to bindings, notes, roles, or order mark existing reports as needing an update.

When automatic binding is enabled, each hint submitted through `add_hint` or `add_task_hint` can include `traffic_refs`; this also works for batched `hints`. The Planner can pass `evidence_hint_id` to the reporting Agent and explicitly select the references for the current task. Inherited hints cannot be cited as the source of a binding. The system does not infer bindings from a hostname, timestamp, or browsing history. Failed submissions do not create partial findings or trigger reports early.

To attach traffic to an existing finding, call `bind_finding_traffic(finding_id, traffic_refs)` without creating the finding again. `list_findings`, `list_task_findings`, `node_detail`, and `get_task_node_detail` return both `finding_id` and `finding_node_id`; the legacy `id` continues to mean the exploration-node ID.

Only optional fields are added to the tool schemas at startup. The reporting Agent receives the existing traffic tools and, by default, the binding tool. Customized binding lists, prompt text, descriptions, and enabled states are retained. Instructions are added after the final tool set is assembled: the reporting role hands off existing evidence, and the reporting Agent verifies, binds, and writes the report. If a platform conversation lacks task context, hand it to the task Agent through structured prompts instead of reporting directly. Existing evidence must be handed off before declaring the task complete, but the workflow need not wait when no traffic was captured. A failed `report_finding` call does not trigger the reporting Agent.

## API

Base path: `/api/exploration/findings/{finding_id}/traffic`, where `finding_id` is the independent finding ID. Existing authentication applies. `context_task` checks task visibility and makes inherited findings read-only.

| Method and path | Request and response |
| --- | --- |
| `GET` | Returns ordered summaries, the evidence version, and the report's adopted evidence version. |
| `POST` | Accepts `{"traffic_refs":[...]}` to add a batch of bindings. |
| `PATCH /{binding_id}` | Accepts `{"version":1,"role":"proof","note":"Reproduces the issue"}`. |
| `DELETE /{binding_id}` | Accepts `{"version":1}`. |
| `PUT /order` | Accepts `{"version":1,"binding_ids":["2","1"]}`; the list must include every binding. |
| `GET /{binding_id}` | Returns snapshot metadata and a bounded text preview. |
| `GET /{binding_id}/body` | Accepts `side`, `offset`, and `length`; `download=1` downloads the complete original bytes. |

Version or ordering conflicts and writes to archived findings return `409`. Writes to inherited findings return `403`. A missing binding or a record that is not a finding returns `404`. Traffic lookup, attachment reading, and verification failures return an explicit error.

## Storage and migration

Startup runs an idempotent PostgreSQL migration that adds `traffic_evidence_snapshots`, `finding_traffic_bindings`, and `findings.evidence_version` / `report_evidence_version` (both default to `0`). Historical text is not used to guess bindings.

Each snapshot stores the original traffic ID, capture time, URL, method, status, request and response headers, body length, and SHA-256. Bodies are stored at `<data>/evidence/blobs/<first two hash characters>/<hash>.bin`, independently of the cleanable `<data>/traffic` directory. Multiple findings can share a snapshot and its bodies. Snapshots cannot be edited; reads and exports fail if integrity verification fails.

The system reads complete traffic text, including large blobs and records from older directories, under the raw-traffic write lock. It writes and verifies the evidence files first, then uses one PostgreSQL transaction for exploration nodes, intent relationships, findings, snapshots, and bindings. The Planner is notified only after the transaction succeeds. A failure can leave an unreferenced file, but cannot create partial business records.

PostgreSQL advisory lock `7337741004` reconciles evidence files with SQL references. A task-row lock prevents archive operations from racing with evidence changes. Recovery holds evidence locks from file installation through metadata submission. Deleting a finding cascades to its bindings.

The cleaner runs hourly. It removes only unreferenced content that is not part of an active operation, and waits at least 24 hours before reclaiming it. Routine traffic cleanup does not touch the evidence directory. Back up PostgreSQL and `data/evidence` together when backing up live data.

## Export and archive

Markdown exports include ordered evidence lists and versions. JSON exports include metadata. CSV exports include counts and binding IDs. `md-zip` includes the finding Markdown and files in this layout:

```text
evidence/<finding_id>/<binding_id>/
  manifest.json
  request.http
  response.http
  request.bin
  response.bin
```

Markdown files link to evidence using relative paths. Before a download is sent, attachments are copied, hashes are verified, the archive is compressed and synced to disk, and every ZIP entry is checked by CRC. Any missing or damaged file fails the entire download. Attachments preserve the original binary bytes.

Archive v3 collects snapshots and bodies through finding-to-binding relationships, without depending on the original traffic database or hostnames. Hot data is cleared only after the archive is verified; evidence shared by other findings is retained. During restore, files are verified before metadata and bindings are restored. Failed restores can be retried. Archives in v1 and v2 remain restorable; missing fields are explicitly set to `0`.

## Validation and limitations

Use a newly created, isolated PostgreSQL database for each test package and set `ARTEX_PG_DSN` to it. This prevents leftover tasks or model fixtures from triggering background runs. Run the relevant package tests and confirm that none were skipped because configuration was missing:

```sh
# Set ARTEX_PG_DSN to an isolated test database before each package run.
go test ./<package> -count=1
go test -race -p 1 ./evidence ./db ./agent ./server -run 'TestEvidence|TestFindingTraffic|TestFindingEvidence|TestReportFindingAtomicContract|TestTaskArchive'
```

Frontend validation includes `npx tsc --noEmit`, Biome checks for affected files, Webpack builds, and `NEXT_EXPORT=1` static exports. Use a separate cache directory to avoid overwriting running development services.

Native end-to-end checks use isolated ports, controlled HTTP/HTTPS targets, and temporary data directories. They cover multiple bindings, cross-page selection, ordering and notes, error messages, read-only inherited findings, downloads and exports after deleting the original traffic, archiving, hot-data cleanup, restore, and hash verification.

The first version uses a global evidence coordination lock. Large batch bindings and exports or slow attachment downloads can make other evidence operations wait. Evidence cannot be reconstructed when no traffic record or complete body was captured. This feature does not change the traffic-capture setting or address HTTPS certificate errors when connecting directly to an IP address.
