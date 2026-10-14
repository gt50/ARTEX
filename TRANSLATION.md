# English translation and remaining Chinese values

Application text, documentation, and ordinary test inputs use English. Some Chinese literals must remain as inputs to compatibility parsers or as data for tests. This guide explains their meanings and records where they remain.

## Legacy system labels

The frontend translates these exact system-generated labels when displaying conversation titles, interception rules, and approval records. Editing a legacy title or rule also starts with its English label. The stored value is retained until an edit is saved. Custom text is preserved.

| Original value | English display |
| --- | --- |
| `新对话` | New conversation |
| `[内置] 删除类接口路径` | [Built-in] Deletion endpoint paths |

The mappings live in `web/src/lib/legacy-system-labels.mjs`. Exact matching avoids changing user-authored text that happens to contain the same words. The database still recognizes the legacy rule name during seeding so upgrades cannot create a duplicate rule.

## Compatibility glossary

These values are recognized as input; replacing them would stop the application from understanding some provider responses, stored records, or older commands.

| Values | Meaning | Use |
| --- | --- | --- |
| `余额不足`, `额度不足`, `额度已用尽`, `欠费` | Insufficient balance, insufficient quota, quota exhausted, overdue payment | Recognize provider billing/quota failures. |
| `备案` | Registration or filing | Recognize ICP registration scope values. |
| `[模型]` | [model] | Recognize the approval source in legacy records and archives. The marker is removed from displayed approval reasons. |
| `实际操作：`, `；成功后的后果：`, `；命中规则：` | Actual operation, consequences after success, matched rule | Parse structured reasons returned under older custom approval prompts. |
| `漏洞`, `资产`, `企业`, `接口` | Finding, asset, company, endpoint | Accept older chat-mention tokens. |
| `应用`, `域名`, `子域名`, `服务` | Application, domain, subdomain, service | Accept older chat-mention tokens. |
| `工具`, `请求审批`, `请求`, `命令` | Tool, requested approval, request, command | Parse older activity summaries; the command label is displayed in English. |
| `意图`, `提示` | Intent, hint | Accept older command aliases alongside the English commands. |
| `失败`, `丢弃`, `拒绝`, `不可达` | Failed, dropped, denied, unreachable | Classify error log messages. |
| `禁用`, `重试` | Disabled, retry | Classify warning log messages. |
| `未登录`, `未授权`, `授权` | Not logged in, unauthorized, authorization | Recognize authentication responses during API reconnaissance. |

Arbitrary text from a provider, user, imported record, or old model answer can also be Chinese. The application does not replace that free text using this glossary: the glossary covers known system values, and cannot translate an entire historical message reliably.

## Historical validation record

Read [the complete English companion](sidequestion/validation-2026-09-10.en.json) for the recorded prompts, answers, activity summaries, and model review. The original [validation record](sidequestion/validation-2026-09-10.json) remains authoritative evidence. IDs, timestamps, assertions, usage, and recorded hashes are retained in the English copy; those hashes still refer to the original transcripts. Quoted page titles in the companion are translations, not byte-for-byte quotations of the original page.

The quoted review's claim that there were no writes is qualified in [the validation notes](sidequestion/VALIDATION.md): the main curl command wrote a local temporary response file. The side-question path executed no tools.

## Test data

Test inputs and their paired expected values use English. UTF-8, token-counting, filename-encoding, and legacy protocol checks remain covered with readable accented text, numeric code points, and explicitly documented protocol constants. See [the English fixture guide](TEST-FIXTURES.en.md) for the conversion details and compatibility exceptions.

The inventory below is a snapshot of Chinese-containing files after this translation pass. Counts are lines containing Chinese, not untranslated interface labels. This guide and the English fixture reference are excluded because the compatibility glossary deliberately quotes the values it explains.

## Inventory (2026-10-13)

### Compatibility inputs

17 files; 30 lines contain Chinese.

| File | Lines |
| --- | ---: |
| [agent/provider.go](<agent/provider.go>) | 1 |
| [db/company_scope.go](<db/company_scope.go>) | 1 |
| [db/db.go](<db/db.go>) | 1 |
| [db/intercept.go](<db/intercept.go>) | 2 |
| [db/schema.sql](<db/schema.sql>) | 1 |
| [db/task_archives_restore.go](<db/task_archives_restore.go>) | 1 |
| [intercept/prompt.go](<intercept/prompt.go>) | 2 |
| [server/chat_mentions.go](<server/chat_mentions.go>) | 3 |
| [server/conversations.go](<server/conversations.go>) | 1 |
| [server/logsink.go](<server/logsink.go>) | 2 |
| [server/server.go](<server/server.go>) | 4 |
| [skills/api-recon/scripts/preload.js](<skills/api-recon/scripts/preload.js>) | 1 |
| [web/src/components/approval-records.tsx](<web/src/components/approval-records.tsx>) | 4 |
| [web/src/components/transcript.tsx](<web/src/components/transcript.tsx>) | 2 |
| [web/src/lib/company-scope.ts](<web/src/lib/company-scope.ts>) | 1 |
| [web/src/lib/legacy-system-labels.mjs](<web/src/lib/legacy-system-labels.mjs>) | 2 |
| [web/src/lib/mock/handler.ts](<web/src/lib/mock/handler.ts>) | 1 |

### Historical evidence

1 files; 15 lines contain Chinese.

| File | Lines |
| --- | ---: |
| [sidequestion/validation-2026-09-10.json](<sidequestion/validation-2026-09-10.json>) | 15 |

### Test inputs

There are no literal Chinese characters in tracked Go or JavaScript test source files. Human-readable inputs and matching expectations use English. The fixture conversion covers prompts, replies, notifications, database records, reports, scopes, and filenames.

Unicode checks remain meaningful: accented English filenames exercise GBK decoding; punctuation and emoji exercise UTF-8 byte and rune limits; numeric code points exercise CJK classification. Dedicated protocol checks use named or explicitly escaped legacy values with English explanations. Escaping these exact wire values does not translate their runtime bytes. See [TEST-FIXTURES.en.md](TEST-FIXTURES.en.md) for the distinction and source links.

## Build validation

The 2026-10-13 Windows build passed with Go 1.26.3 and Node.js 22.23.3: TypeScript checking, eleven frontend tests, 31 statically exported pages, and the Go binary with embedded UI. The final PostgreSQL integration run passed all 17 root-module packages using a fresh isolated database per package. The bundled Norma suite passed all 16 packages (741 passing tests, seven skips, zero failures).

The fixture conversion updated ordinary inputs and their paired assertions in 56 test files. UTF-8 coverage now uses English with multibyte punctuation, accents, and emoji. Approval-review fixtures follow the English response contract. ICP examples and GBK filenames also use English; numeric Unicode checks and exact escaped legacy protocol checks preserve specialized coverage. Background-task integration tests wait for completion rather than assuming a fixed startup time.

The failure-repair pass added Windows write-through evidence publication while retaining directory syncing on other platforms, waited for background processes to release output files before cleanup, connected provider context-window learning, and corrected compression pacing and summary consolidation. Tests now use the current compact overview contract, current English errors, platform-appropriate shell syntax, and a portable archive-write failure fixture. Regression checks cover publication failures, cadence scaling, the usage threshold, and summary consolidation.

## Visual and runtime audit

All 13 README screenshots were recaptured from the English frontend with demo data. The author banner now reads “WeChat Search”; its QR payload was decoded and verified to match the original. The 24 main frontend routes rendered without Chinese text during a local demo audit. Login and setup redirect in demo mode; their source and production exports are English. The audit also repaired missing notification demo metadata and added two regression tests for its response shape and read-only behavior.

A Go syntax-tree audit found and translated 16 additional diagnostic fragments, log messages, and skip reasons. Fixture inputs are retained and explained in the English reference.

### Author banner provenance

Saved asset: `screenshots/wx.png`. The English banner was edited with the built-in imagegen tool. Final cleanup prompt:

> Clean up only the unwanted ragged black marks and distressed/noisy edges along the top and bottom of this English author banner. It should be a pristine flat digital graphic: a smooth white rounded rectangle, clean fully transparent corners and exterior, no black outline or border, no distressing, no paper texture or grain. Keep everything inside the panel unchanged, especially the exact QR code modules and center logo, the green WeChat icon, exact text 'WeChat Search' and 'SecSentry', the green panel and magnifier. QR must retain its identical scannable payload. Do not redraw its modules or alter its geometry.

The result was checked visually, and both the original and translated QR codes were decoded with ZXing to confirm that their payloads match.
