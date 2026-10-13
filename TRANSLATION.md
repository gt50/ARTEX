# English translation and remaining Chinese values

Application text and documentation are being converted to English. Some Chinese literals must remain as inputs to compatibility parsers or as data for tests. This guide explains their meanings and records where they remain.

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

Chinese input in tests exercises Unicode byte budgets, token estimates, multilingual parsing, ICP values, legacy labels, filenames, stored messages, and archive recovery. Keep these inputs and their paired expected values consistent. Comments and test-failure messages can be translated without changing the input being tested.

The inventory below is a snapshot of Chinese-containing files after this translation pass. Counts are lines containing Chinese, not untranslated interface labels. This guide is excluded because it deliberately quotes the values it explains.

## Inventory (2026-10-13)

### Compatibility inputs

18 files; 31 lines contain Chinese.

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
| [web/src/components/exploration-graph.tsx](<web/src/components/exploration-graph.tsx>) | 1 |
| [web/src/components/transcript.tsx](<web/src/components/transcript.tsx>) | 2 |
| [web/src/lib/company-scope.ts](<web/src/lib/company-scope.ts>) | 1 |
| [web/src/lib/legacy-system-labels.mjs](<web/src/lib/legacy-system-labels.mjs>) | 2 |
| [web/src/lib/mock/handler.ts](<web/src/lib/mock/handler.ts>) | 1 |

### Historical evidence

1 files; 15 lines contain Chinese.

| File | Lines |
| --- | ---: |
| [sidequestion/validation-2026-09-10.json](<sidequestion/validation-2026-09-10.json>) | 15 |

### Test data and case labels

68 files; 401 lines contain Chinese.

| File | Lines |
| --- | ---: |
| [agent/blackboard_inheritance_test.go](<agent/blackboard_inheritance_test.go>) | 1 |
| [agent/prompt_now_test.go](<agent/prompt_now_test.go>) | 6 |
| [agent/prompt_test.go](<agent/prompt_test.go>) | 8 |
| [agent/provider_quota_test.go](<agent/provider_quota_test.go>) | 1 |
| [agent/review_context_test.go](<agent/review_context_test.go>) | 4 |
| [agent/terminalreason_test.go](<agent/terminalreason_test.go>) | 4 |
| [agent/tools_overview_test.go](<agent/tools_overview_test.go>) | 1 |
| [db/activity_page_test.go](<db/activity_page_test.go>) | 2 |
| [db/company_scope_consistency_test.go](<db/company_scope_consistency_test.go>) | 2 |
| [db/company_scope_test.go](<db/company_scope_test.go>) | 10 |
| [db/config_test.go](<db/config_test.go>) | 3 |
| [db/exploration_test.go](<db/exploration_test.go>) | 2 |
| [db/finding_assets_test.go](<db/finding_assets_test.go>) | 9 |
| [db/finding_retests_test.go](<db/finding_retests_test.go>) | 3 |
| [db/findings_test.go](<db/findings_test.go>) | 13 |
| [db/intent_delete_test.go](<db/intent_delete_test.go>) | 20 |
| [db/intercept_detail_test.go](<db/intercept_detail_test.go>) | 4 |
| [db/intercept_filter_test.go](<db/intercept_filter_test.go>) | 2 |
| [db/notification_test.go](<db/notification_test.go>) | 30 |
| [db/task_assets_test.go](<db/task_assets_test.go>) | 4 |
| [db/task_context_unit_test.go](<db/task_context_unit_test.go>) | 1 |
| [db/tasks_test.go](<db/tasks_test.go>) | 4 |
| [intercept/prompt_test.go](<intercept/prompt_test.go>) | 15 |
| [intercept/review_context_test.go](<intercept/review_context_test.go>) | 14 |
| [intercept/trace_test.go](<intercept/trace_test.go>) | 2 |
| [norma/harness/settlement_test.go](<norma/harness/settlement_test.go>) | 2 |
| [norma/harness/timeout_settle_test.go](<norma/harness/timeout_settle_test.go>) | 2 |
| [norma/noa/compress_test.go](<norma/noa/compress_test.go>) | 2 |
| [norma/noa/edge_test.go](<norma/noa/edge_test.go>) | 3 |
| [norma/noa/fuzz_test.go](<norma/noa/fuzz_test.go>) | 4 |
| [norma/noa/nudgetext_test.go](<norma/noa/nudgetext_test.go>) | 1 |
| [norma/noa/prune_test.go](<norma/noa/prune_test.go>) | 2 |
| [norma/noa/tokenize_test.go](<norma/noa/tokenize_test.go>) | 5 |
| [norma/noaadapter/project_test.go](<norma/noaadapter/project_test.go>) | 1 |
| [notify/channels_test.go](<notify/channels_test.go>) | 13 |
| [notify/email_test.go](<notify/email_test.go>) | 7 |
| [notify/filter_test.go](<notify/filter_test.go>) | 13 |
| [notify/pack_test.go](<notify/pack_test.go>) | 13 |
| [notify/redact_test.go](<notify/redact_test.go>) | 2 |
| [notify/render_test.go](<notify/render_test.go>) | 8 |
| [notify/webhook_template_test.go](<notify/webhook_template_test.go>) | 3 |
| [selfupdate/selfupdate_test.go](<selfupdate/selfupdate_test.go>) | 2 |
| [server/assets_scope_test.go](<server/assets_scope_test.go>) | 2 |
| [server/auth_test.go](<server/auth_test.go>) | 2 |
| [server/chat_mentions_test.go](<server/chat_mentions_test.go>) | 18 |
| [server/core_test.go](<server/core_test.go>) | 1 |
| [server/engine_emptyturn_test.go](<server/engine_emptyturn_test.go>) | 12 |
| [server/finding_retests_test.go](<server/finding_retests_test.go>) | 8 |
| [server/finding_traffic_test.go](<server/finding_traffic_test.go>) | 2 |
| [server/findings_groups_test.go](<server/findings_groups_test.go>) | 1 |
| [server/intercept_detail_test.go](<server/intercept_detail_test.go>) | 1 |
| [server/intercept_filter_test.go](<server/intercept_filter_test.go>) | 2 |
| [server/intercept_live_test.go](<server/intercept_live_test.go>) | 8 |
| [server/intercept_review_test.go](<server/intercept_review_test.go>) | 3 |
| [server/mgmt_test.go](<server/mgmt_test.go>) | 2 |
| [server/notify_api_test.go](<server/notify_api_test.go>) | 33 |
| [server/skill_upload_test.go](<server/skill_upload_test.go>) | 21 |
| [server/task_llm_test.go](<server/task_llm_test.go>) | 1 |
| [server/task_metadata_test.go](<server/task_metadata_test.go>) | 1 |
| [server/task_templates_test.go](<server/task_templates_test.go>) | 3 |
| [server/trigger_merge_test.go](<server/trigger_merge_test.go>) | 6 |
| [server/update_test.go](<server/update_test.go>) | 1 |
| [sidequestion/context_test.go](<sidequestion/context_test.go>) | 4 |
| [traffic/reclaim_test.go](<traffic/reclaim_test.go>) | 3 |
| [traffic/traffic_store_test.go](<traffic/traffic_store_test.go>) | 9 |
| [traffic/upgrade_test.go](<traffic/upgrade_test.go>) | 4 |
| [web/src/lib/chat-mentions.test.mjs](<web/src/lib/chat-mentions.test.mjs>) | 2 |
| [web/src/lib/legacy-system-labels.test.mjs](<web/src/lib/legacy-system-labels.test.mjs>) | 3 |

Developer comments and direct failure messages were translated in 44 Go test files. Another 44 assertions were updated to match English system output. Translated 107 human-readable test case labels in a further pass. Fixture inputs and diagnostic formatting directives were retained. Full-build validation also repaired the metering test setup so its usage assertions run against a newly created isolated database.

## Build validation

The 2026-10-13 Windows build passed with Go 1.26.3 and Node.js 22.23.3: TypeScript checking, nine frontend tests, 31 statically exported pages, and the Go binary with embedded UI. The final PostgreSQL integration run passed all 17 root-module packages using a fresh isolated database per package. The bundled Norma suite passed all 16 packages (741 passing tests, seven skips, zero failures).

The failure-repair pass added Windows write-through evidence publication while retaining directory syncing on other platforms, waited for background processes to release output files before cleanup, connected provider context-window learning, and corrected compression pacing and summary consolidation. Tests now use the current compact overview contract, current English errors, platform-appropriate shell syntax, and a portable archive-write failure fixture. Regression checks cover publication failures, cadence scaling, the usage threshold, and summary consolidation.
