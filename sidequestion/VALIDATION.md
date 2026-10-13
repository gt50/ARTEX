# `/btw` Verification record

Date: 2026-09-10. Branch: `codex/btw-side-question`. Baseline: `8dae851b9b622f2ff2631f332fde9719d0b16fba`.

Use a standalone PostgreSQL test library and data directory; real model credentials are only injected into the standalone test environment, no code or this record is written, and the product default model is not modified. Go 1.26.3, norma v0.3.6, Next.js 16.2.9.

The actual model conversations, return objects, engineering assertions, and Qwen original review text are saved in [validation-2026-09-10.json](validation-2026-09-10.json), which does not have API credentials. An [English reading copy](validation-2026-09-10.en.json) translates the recorded prompts, answers, and review; the original remains the authoritative evidence.

## Project inspection

| Scope | Results | Evidence |
| --- | --- | --- |
| Deep copy of structured messages and tool parameters | Pass | `TestCheckpointDeepCopyAndBoundaries` |
| Summary/compression requests are excluded; complete replies and final states are captured; partial replies are excluded | Pass | `TestCheckpointDeepCopyAndBoundaries`, `TestSnapshotExcludesPartialStreamAndSelectsPoolMember` |
| The snapshot records the model actually selected from the pool | Pass | `TestSnapshotExcludesPartialStreamAndSelectsPoolMember` |
| Tool-call pairing, replay of 20 pairs, budget trimming, and over-budget errors | Pass | `TestBuildRequestCompactionToolPairingAndBudget` |
| Side questions run in parallel with the main session and remain isolated in both directions | Pass | Blocking Provider, `TestMainSideConcurrencyAndIndependentCancellation` |
| No tool execution; streaming and non-streaming behavior; usage retained on failure | Pass | `TestServiceNoToolsAndUsageOnFailure` |
| Real norma ChatAgent with a local Read tool; main transcript and activity remain isolated | Pass | `TestSideActualChatCheckpointToolResultAndTranscriptIsolation`, streaming and non-streaming cases |
| Persistence, paging, idempotency, and recovery of partial answers after restart | Pass | `TestSideHistoryIdempotencyPagingAndRecovery` |
| Empty and late write contention, parent resource deletion, version comparison | Pass | `TestSideClearLateWritersAndDeletedParent` |
| MainAgent / Worker Archiving and Recovery, v1/v2/v3 | Pass | `TestSideTaskArchiveVersions` |
| Three parent interfaces, authentication, resource ownership, Worker logical deletion | Passed | `TestSideHTTPGlobalLimitTaskWorkerAndDeletion`, `TestSideCheckpointPersistsBeforeAdmissionAndRestart` |
| Side questions during a busy main session; independent SSE reconnect/disconnect, cancellation, and clearing | Pass | `TestSideHTTPBusyIsolationClearAndReconnect` |
| 1 per parent session / 4 globally concurrency | Pass | Two `TestSideHTTP…` use cases |
| Submission is rejected when the snapshot is missing or stale; an old session cannot be forged | Pass | `TestSideCheckpointPersistsBeforeAdmissionAndRestart` |
| Requests are rejected if the cached configuration is deleted or the model changes | Pass | `TestSideRejectsDeletedOrChangedCachedProfile` |
| Cancel before archiving and wait for final answer and usage drop | Passed | `TestSideTaskDrainPersistsBeforeArchive` |
| When a streaming consumer cancels in advance, only one usage and bypass attribution will be recorded | Pass | `TestSideUsageRecordedOnceOnConsumerCancellation` |
| A restored Worker runtime resumes publishing snapshots | Pass | `TestSideRestoredWorkerRuntimePublishesNewCheckpoint` |
| Race checks for related packages | Pass | Command below |
| TypeScript checks and production builds | Pass | `npx tsc --noEmit`, `npm run build` |
| Biome checks for the three new frontend modules | Pass | `biome check` |

The automated check can be reproduced after configuring `ARTEX_PG_DSN` in a separate droppable database (do not point to the production database):

```sh
go test -race ./agent ./db ./server ./sidequestion ./llmrec ./llmpool \
  -run 'Test(Side|Checkpoint|Snapshot|BuildRequest|Service|MainSide|CaptureRun|TaskArchive|CompleteForwards|StopIntent|CancelIntent)' -count=1
cd web
npx tsc --noEmit
npx biome check src/lib/side-questions.ts src/hooks/use-side-questions.ts src/components/side-question-workspace.tsx
npm run build
```

Full Go return is not fully green: Two existing tests of the `server` package failed in the temporary directory cleanup phase, and both reported `TempDir RemoveAll … directory not empty`:

- `TestInheritedActivityDetailAndRelationDeletion`
- `TestTaskMetadataPatchReturnsRenameAndPin`

After exporting the source code from the above unmodified baseline, re-running the `server` package in the same isolation environment also reproduced the two cleanup failures. The baseline run additionally failed the target node number assertion for `TestCoreTaskLifecyclePG`; the final modified `server` regression did not fail the assertion. Other packages passed, and this bypass-related use case and race check passed. The baseline issues are not marked as passed for this acceptance, nor are the existing assertions modified for hidden issues.

Next.js build outputs existing multiple lockfile / workspace root inference warnings; build completes and all pages are generated successfully.

## Browser check

Use the Codex In-app Browser to connect a standalone local Go service and a Next.js development server. Desktop and 390 × 844 narrow screen complete the following manual automation operations, check screenshots and browser logs:

- When normal chat is running, enter `/btw`, and the main content and bypass will be displayed at the same time; the desktop sidebar will be normal.
- Continuous questioning; retain the generated part after the bypass is stopped; the main process continues.
- The request continues when the panel is closed, and the completed answer is restored after reopening; the empty `/btw` restore history after refreshing the page.
- The input, buttons, history, and closing operations of the narrow-screen Drawer are normal, and there is no horizontal overflow.
- Clear the usage confirmation pop-up window. After clearing, the history disappears and the main transcript and snapshots are retained.
- The task MainAgent and the two Workers asked and switched separately, and the Agent labels and history were not cross-talked.
- The blocking local model fixture keeps the Worker running; submit `/btw` from the Worker main input box. After stopping the bypass, the Worker still displays real-time running and its own pause button, and the bypass saves partial answers.
- The browser error/warning log is empty.

Controllable fixtures are used to accurately verify concurrent timing without relying on the output speed of the real model. During debugging, the two Worker runtime checks did not form a valid concurrency window (task ended/answer ended early), correct the fixture and redo it and pass; these initial operations will not be recorded as valid passes.

## Real model dialogue

Prioritize detection of `grok-4.6` and OpenAI compatible interface `http://127.0.0.1:12580/tingly/openai`. Detecting HTTP 200 returns the model names `grok-4.6` and `READY`, which takes 2.82 seconds. The first choice is available, so the Tingly `glm` or Zhipu `glm-5.3` backup chains are not enabled; these two backup services are not verified this time.

| Scenario | Actual results |
| --- | --- |
| Asking for assets, targets, tags while main session is running | Return `redhaze.top`, home page read and summary targets, `BTW-REAL-0910`; bypass complete, 16.97 seconds |
| After the main session completes reading the homepage, ask the tool for the basis | Correctly quote WebFetch 200, curl jump 301 → 302 → 200, page title; 7.24 seconds |
| Bypass requires Bash to create test file | Execution refused, target file not created; 7.74 seconds |
| The completed bypass does not change the main context | The main transcript SHA-256 is consistent with the main activity record; the number of bypass tool executions is 0 |
| Question after actually stopping/restarting the Go service | Keep the previous 3 bypass histories, answer assets, tags and titles directly from the persistent snapshot, without re-running the main Agent |
| New session using Grok non-streaming configuration | Answer assets and `ATOMIC-0910` correctly; return and save usage: input 11734, output 138, cache_read 11520 |

The main session of the asset case uses WebFetch and Bash/curl to read the public homepage. The landing page is `https://id.redhaze.top/home`, and the title is "RedHaze Technology RedHaze Group · Global Comprehensive Group Portal". Bash temporarily stores the response in the local test file; no writing is performed remotely. This fact is verified separately from "Bypass has no execution tools".

Main transcript check value: `e7e61f135a4a120954b539f357e8c4205d7d5cd7460dcaf3dc0fd066463e1d00`.

**Usage Limit:** Tingly's Grok streaming response does not return usage. Separately send `stream_options.include_usage=true` verification directly, HTTP 200, 12 data frames, 0 usage frames. Therefore a 0 in a streaming test indicates that the endpoint is not providing usage and cannot be interpreted as no billing. Non-flowing usage and failed/cancelled usage of fixtures are saved correctly.

## Qwen Review

Review model `qwen-flash`, OpenAI compatible interface `https://dashscope.aliyuncs.com/compatible-mode/v1`, HTTP 200. The first three real bypass conversations, the main session tool basis and the engineering assertions are provided; `verdict: accept` and `concerns: []` are returned, and the answers are considered to be consistent with the assets, tags, and page read evidence, and the bypass tool refuses to comply with the constraints. Review usage: prompt 6625, completion 312, total 6937.

The scope of this Qwen review does not include service restarts and non-streaming tests that were added later. Qwen's generalization of "no writes" is too broad: the main session curl does create local response temp files, which is explicitly documented above. Concurrency, zero-tool execution, and transcript isolation are judged by engineering assertions, and model review only assists in assessing answer quality.
