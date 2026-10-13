# Long side questions and context budgeting

Fixed on 2026-09-11. The original implementation treated the character count of the serialized request as a token count and inherited the main task's 32K output reservation. As a result, side questions could be rejected prematurely when the context contained large HTML, JavaScript, or tool results.

## Open-source implementation references

- [Grok CLI side-question context](https://github.com/superagent-ai/grok-cli/blob/fb97af83f06dca873281d60168430f06c8de6324/src/agent/agent.ts#L739) extracts recent user and assistant text, with an overall budget of about 2,000 characters and a maximum of 400 characters per excerpt. This path does not keep a continuous side-question history.
- [Grok CLI standalone request](https://github.com/superagent-ai/grok-cli/blob/fb97af83f06dca873281d60168430f06c8de6324/src/utils/side-question.ts) uses an independent cancellation signal, sets a 2,048-token output limit when supported, and provides no tools.
- [Grok CLI main-session compaction](https://github.com/superagent-ai/grok-cli/blob/fb97af83f06dca873281d60168430f06c8de6324/src/agent/compaction.ts) estimates token usage, retains recent content, rolls older content into summaries, and handles truncation across turns.
- [OpenCode session compaction](https://github.com/anomalyco/opencode/blob/b3f1a96c6dd7adeb28b36dd11add1998fc84d67b/packages/core/src/session/compaction.ts) estimates the full request, reserves output and buffer space, retains recent content with a rolling summary, and uses a tool-free summary request. Its defaults are an 8,000-token recent-content budget and a 4,096-token summary output limit.
- [OpenCode overflow recovery](https://github.com/anomalyco/opencode/blob/b3f1a96c6dd7adeb28b36dd11add1998fc84d67b/packages/core/src/session/runner/llm.ts) retries an overflow only before assistant output starts. The retried request is not sent through the same recovery path again.

ARTEX uses a separate output budget, recent context, rolling summaries, and limited overflow recovery. It preserves norma v0.3.6 structured messages and tool-call pairing. It does not use Grok's text excerpts or write OpenCode-style compaction events into the main ARTEX transcript.

## Request budget and execution

- Input size is estimated using norma's UTF-8 byte estimate with a 4/3 margin per content block. System prompts, tool schemas, and message framing are also included. This is an estimate, not an exact model token count.
- Side-question output defaults to at most 8,192 tokens and never exceeds the output limit in the main configuration. The available environment variable `ARTEX_BTW_MAX_OUTPUT_TOKENS` can set a limit from 256 to 32,768. It does not change the product's default model or main-task parameters.
- The input budget is the context window minus the output limit and a safety margin. If the model's window is unknown, the platform default of 200K is used. The margin is 5% of the window, with a minimum of 128 and a maximum of 8,192 tokens.
- Up to 20 successful question-and-answer pairs are loaded per batch, in chronological order. At most 20 original pairs are retained, using no more than one quarter of the input budget or 16K tokens, whichever is lower.
- Older pairs beyond that limit are folded into a rolling summary. The summary records historical sources and context time. Past assistant answers are not treated as fresh tool evidence; when they conflict with the latest main snapshot, the snapshot takes precedence.
- If the main context is still too large, only its older messages are summarized in the side-question copy. Up to 8K tokens of recent context are retained. Trimming never separates a tool call from its result, and an individually oversized pair is summarized as a whole.
- Summary input is split safely on UTF-8 boundaries to fit the remaining context window. Each summary request has a 2,048-token output limit. Empty, truncated, over-budget, or tool-calling summaries are not cached. A side question allows at most 12 summary calls, all within the same 120-second timeout; reaching the limit fails explicitly instead of looping.
- If the first model request exceeds the context limit before emitting text or tool calls, it may be retried once with a smaller context. Recovery stops if the estimated size did not decrease. Other model errors and requests that have already streamed output are not retried this way.
- Usage from summaries, failed attempts, and canceled requests is accumulated under the same side-question request. If a provider does not return usage, the recorded amount remains zero; estimates are not presented as actual usage.

## Persistence and interface

`side_question_sessions.memory` stores rolling summaries of older question-and-answer pairs, the cleared-through ordinal, and main-context summaries cached by snapshot identity. `side_question_requests.context_info` stores the preparation phase, number of replayed pairs, summary usage, and budget estimate.

A summary is saved only while its original request is still running and the cleared-through version still matches. Clearing also clears the cache, and late writes cannot restore cleared data. New snapshots do not reuse summaries cached for older snapshots. Summary data is included in v3 task archives. Missing fields in older v3 archives are initialized as empty objects; v1 and v2 remain supported.

The POST endpoint returns the request before preparation and compaction begin in the background. Admission locks and database transactions are not held during that work. SSE and history show preparation, Q&A, compaction, and answer phases. A compaction failure is saved as the request's failed final state. The frontend retains the error and restores the failed question's draft without a toast blocking the input; history polling no longer clears submission errors.

## Verification record

- Automated checks passed for replaying 19, 20, 21, and 50 pairs; preserving older conclusions across 20 pairs; and reusing the summary cache after restart.
- Checks passed for very long Chinese answers and code contexts, chunked request budgets, tool-call pairing, unchanged snapshots, and cache invalidation when a new snapshot arrives.
- Checks passed for summary failure, cancellation, truncation, oversized and tool-calling summary results, clear races, the summary-call limit, one-time overflow recovery, and flows that must not retry.
- Standalone PostgreSQL checks passed for paging, restart behavior, v1/v2/v3 archives, summary-cache and budget-metadata recovery, v3 archives missing new fields, and 20 parent sessions sharing a global limit of four concurrent requests.
- The Go candidate service built. TypeScript checks, Biome checks, and production builds for the frontend passed in separate directories.
- Browser checks used standalone UI fixtures at 1280×720 and 390×844. They covered summary-phase progress, range prompts, draft recovery after failure, unobtrusive errors, no horizontal overflow, and no console errors. Temporary clamps were removed.
- Read-only replay of existing Worker snapshots on this machine passed the new budget check. For example, a 293,085-character snapshot for Worker #3 is no longer rejected based on a mistaken raw character count. This check made no external model calls.
- An automatic approval review rejected a proposed real-model test that would send private Worker snapshots to Grok. The test was not run and is not counted as passing.
- On 2026-09-11 at 00:37, the local backend was restarted as requested, using the original database, data directory, and login configuration. The running binary matches the candidate binary by SHA-256; `/api/health` returned normally for both backend and frontend agents.

The checks used a separate test database:

```sh
go test -race ./sidequestion ./db ./server -run 'TestSide|TestCheckpoint|TestSnapshot|TestBuildRequest|TestService|TestMainSide|TestTaskArchive' -count=1
go build ./cmd/artex
npx tsc --noEmit
npm run build -- --webpack
```

Frontend production builds used a separate copy to avoid overwriting the previewed `.next` directory. The candidate service was placed at `/private/tmp/artex-btw-budget-candidate` and copied to `/private/tmp/artex-btw-preview/artex` for startup. The original binary backup was `artex.before-context-budget` in the same directory.
