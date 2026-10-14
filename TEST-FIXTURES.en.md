# English reference for Chinese compatibility fixtures

Ordinary test inputs, simulated model replies, task names, and expected results now use English. Generic UTF-8 tests use English text with accented Latin characters, Unicode punctuation, and emoji. The remaining Chinese literals specifically exercise legacy label parsing, provider quota errors, ICP registrations, CJK token counting, or Chinese filename and GBK decoding. Their English meanings are listed below; changing these literals would change the behavior being tested.

Quoted commands, requests, HTML, SQL, and policy text are test data. They are displayed as escaped text and are not instructions to execute. Format directives and hidden Unicode controls are visible in the original-input column.

The catalog was extracted from Go syntax trees on 2026-10-13 and covers 52 occurrences of 44 distinct literals in 11 files. It includes rune literals. TypeScript mention aliases and legacy system labels are explained in [TRANSLATION.md](TRANSLATION.md).

| Original input (JSON escaped) | English meaning | Source |
| --- | --- | --- |
| `"京 ICP 备 %d 号"` | `"Beijing ICP No. %d"` | [agent/blackboard_inheritance_test.go:42](agent/blackboard_inheritance_test.go#L42) |
| `"账户余额不足，请充值"` | `"The account balance is insufficient, please top up"` | [agent/provider_quota_test.go:27](agent/provider_quota_test.go#L27)<br>[server/task_llm_test.go:120](server/task_llm_test.go#L120) |
| `"京 ICP备 123号"` | `"Beijing ICP No. 123"` | [db/company_scope_test.go:59](db/company_scope_test.go#L59) |
| `"京icp备123号"` | `"Beijing ICP No. 123"` | [db/company_scope_test.go:59](db/company_scope_test.go#L59) |
| `"沪网备案 9988"` | `"Shanghai Net Registration 9988"` | [db/company_scope_test.go:60](db/company_scope_test.go#L60) |
| `"沪网备案9988"` | `"Shanghai Net Registration 9988"` | [db/company_scope_test.go:60](db/company_scope_test.go#L60) |
| `"备案 www.example.com"` | `"Registration www.example.com"` | [db/company_scope_test.go:65](db/company_scope_test.go#L65)<br>[db/company_scope_test.go:65](db/company_scope_test.go#L65) |
| `"某公司 ICP v1.0"` | `"A company ICP v1.0"` | [db/company_scope_test.go:66](db/company_scope_test.go#L66) |
| `"某公司 icp v1.0"` | `"A company icp v1.0"` | [db/company_scope_test.go:66](db/company_scope_test.go#L66) |
| `"备案 例．com"` | `"Filing case. com"` | [db/company_scope_test.go:67](db/company_scope_test.go#L67)<br>[db/company_scope_test.go:67](db/company_scope_test.go#L67) |
| `" 京ICP 备 123号-1\t"` | `"Beijing ICP No. 123-1"` | [db/company_scope_test.go:212](db/company_scope_test.go#L212) |
| `"京icp备123号-1"` | `"Beijing ICP No. 123-1"` | [db/company_scope_test.go:216](db/company_scope_test.go#L216) |
| `"京 ICP备 998877号"` | `"Beijing ICP No. 998877"` | [db/company_scope_test.go:268](db/company_scope_test.go#L268) |
| `"京icp备998877号"` | `"Beijing ICP No. 998877"` | [db/company_scope_test.go:276](db/company_scope_test.go#L276) |
| `" 京 ICP备 998877号 "` | `"Beijing ICP No. 998877"` | [db/company_scope_test.go:288](db/company_scope_test.go#L288) |
| `"[模型] 放行"` | `"[model] Allow"` | [db/intercept_filter_test.go:15](db/intercept_filter_test.go#L15) |
| `" 京 ICP 备 12345678 号-1 "` | `"Beijing ICP No. 12345678-1"` | [db/task_assets_test.go:29](db/task_assets_test.go#L29) |
| `"Actual operation: Read a file；成功后的后果：返回内容；命中规则：A5"` | `"Actual operation: Read a file; consequences after success: return content; matched rule: A5"` | [intercept/prompt_test.go:35](intercept/prompt_test.go#L35) |
| `"实际操作："` | `"Actual operation:"` | [intercept/prompt_test.go:88](intercept/prompt_test.go#L88) |
| `"；成功后的后果：Save the file only；命中规则：A2"` | `"; Consequence if successful: Save the file only; Matched rule: A2"` | [intercept/prompt_test.go:88](intercept/prompt_test.go#L88) |
| `"中文"` | `"Chinese"` | [norma/noa/tokenize_test.go:15](norma/noa/tokenize_test.go#L15) |
| `"你好世界"` | `"Hello World"` | [norma/noa/tokenize_test.go:16](norma/noa/tokenize_test.go#L16) |
| `"认证方案 auth.go"` | `"Authentication scheme auth.go"` | [norma/noa/tokenize_test.go:20](norma/noa/tokenize_test.go#L20) |
| `"中文测试"` | `"Chinese test"` | [norma/noa/tokenize_test.go:38](norma/noa/tokenize_test.go#L38) |
| `"中"` | `"Middle/Chinese (single Unicode character)"` | [norma/noa/tokenize_test.go:57](norma/noa/tokenize_test.go#L57) |
| `"文"` | `"Text (single Unicode character)"` | [norma/noa/tokenize_test.go:57](norma/noa/tokenize_test.go#L57) |
| `"[\n\t\t\"example.com\",\n\t\t{\"kind\":\"icp\",\"value\":\"京 ICP备 123号\"},\n\t\t{\"kind\":\"keyword\",\"value\":\"Acme Security\"}\n\t]"` | `"Scope inputs: example.com; an ICP filing value \"Beijing ICP filing No. 123\"; and the keyword \"Acme Security\"."` | [server/assets_scope_test.go:18](server/assets_scope_test.go#L18) |
| `"@[漏洞#12 old label]"` | `"@[bug#12 old label]"` | [server/chat_mentions_test.go:41](server/chat_mentions_test.go#L41) |
| `"中文技能"` | `"Chinese skill"` | [server/skill_upload_test.go:21](server/skill_upload_test.go#L21)<br>[server/skill_upload_test.go:173](server/skill_upload_test.go#L173)<br>[server/skill_upload_test.go:176](server/skill_upload_test.go#L176)<br>[server/skill_upload_test.go:239](server/skill_upload_test.go#L239) |
| `"端口扫描-x"` | `"port scan -x"` | [server/skill_upload_test.go:21](server/skill_upload_test.go#L21) |
| `"日本語スキル"` | `"Japanese skill"` | [server/skill_upload_test.go:21](server/skill_upload_test.go#L21) |
| `"中文 技能"` | `"Chinese skill"` | [server/skill_upload_test.go:23](server/skill_upload_test.go#L23) |
| `"中文/技能"` | `"Chinese/Skills"` | [server/skill_upload_test.go:24](server/skill_upload_test.go#L24) |
| `"中\u202e文"` | `"The characters for \"Chinese\" with a right-to-left override between them."` | [server/skill_upload_test.go:24](server/skill_upload_test.go#L24) |
| `"中文技能!"` | `"Chinese skill!"` | [server/skill_upload_test.go:25](server/skill_upload_test.go#L25) |
| `"参考/中文说明.md"` | `"Reference/Chinese description.md"` | [server/skill_upload_test.go:43](server/skill_upload_test.go#L43)<br>[server/skill_upload_test.go:43](server/skill_upload_test.go#L43) |
| `"assets/图片-1_v2.png"` | `"assets/image-1_v2.png"` | [server/skill_upload_test.go:46](server/skill_upload_test.go#L46)<br>[server/skill_upload_test.go:46](server/skill_upload_test.go#L46) |
| `"中文 名.md"` | `"Chinese name.md"` | [server/skill_upload_test.go:56](server/skill_upload_test.go#L56) |
| `"中\u202e文.md"` | `"Filename containing the characters for \"Chinese\", a right-to-left override, and .md."` | [server/skill_upload_test.go:56](server/skill_upload_test.go#L56) |
| `"---\nname: 中文技能\ndescription: Test skill\n---\nBody\n"` | `"---\nname: Chinese skill\ndescription: Test skill\n---\nBody\n"` | [server/skill_upload_test.go:129](server/skill_upload_test.go#L129) |
| `"中文技能/SKILL.md"` | `"Chinese skill/SKILL.md"` | [server/skill_upload_test.go:166](server/skill_upload_test.go#L166) |
| `"中文技能/参考资料.md"` | `"Chinese skill/reference materials.md"` | [server/skill_upload_test.go:167](server/skill_upload_test.go#L167) |
| `"参考资料.md"` | `"References.md"` | [server/skill_upload_test.go:176](server/skill_upload_test.go#L176) |
| `"---\nname: \"中文技能\"\ndescription: x\n---\n"` | `"Skill metadata: name \"Chinese skill\", description \"x\"."` | [server/skill_upload_test.go:238](server/skill_upload_test.go#L238) |

## Frontend fixture labels

| Original input | English meaning | Source |
| --- | --- | --- |
| `新对话` | New conversation. | [web/src/lib/legacy-system-labels.test.mjs:6](web/src/lib/legacy-system-labels.test.mjs#L6) |
| `[内置] 删除类接口路径` | [Built-in] Deletion endpoint paths. | [web/src/lib/legacy-system-labels.test.mjs:7](web/src/lib/legacy-system-labels.test.mjs#L7) |
| `新对话 about a custom task` | New conversation about a custom task. This user-authored label is intentionally preserved rather than treated as the exact legacy system title. | [web/src/lib/legacy-system-labels.test.mjs:11](web/src/lib/legacy-system-labels.test.mjs#L11) |
