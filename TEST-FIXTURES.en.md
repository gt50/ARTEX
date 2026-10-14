# English test inputs and compatibility checks

Human-readable test inputs and their expected results use English. This includes prompt templates, simulated replies, task and finding names, notification text, report bodies, search terms, filenames, and ICP registration examples.

## Unicode coverage

Generic UTF-8 tests use English text with accented Latin characters, Unicode punctuation, and emoji. Byte and rune limits still exercise multibyte characters at every cut point.

- GBK archive decoding uses the accented English filename `café-skill`. The accent encodes as multibyte GBK data, so the test still verifies decoding rather than just accepting ASCII.
- CJK token classification uses numeric Unicode code points. Their test names describe the character ranges in English; no knowledge of the language is needed.
- Invalid filenames use English words with explicit nonbreaking-space or directional-control escapes.
- ICP examples use `Beijing ICP No 123` and `Shanghai ICP Registration 9988`, with matching normalized expectations.

## Exact legacy protocols

Two compatibility checks deliberately preserve older encoded protocol data. Their meanings and assertions are in English:

| Check | English meaning | Source |
| --- | --- | --- |
| Reject a mixed approval-response format | An English actual-operation heading must not be mixed with older consequence and matched-rule headings. The old headings are written as Unicode escapes. | [Approval verdict tests](intercept/prompt_test.go) |
| Translate old system labels | The named constants `legacyConversationTitle` and `legacyDeletionRuleTitle` contain escaped old labels meaning “New conversation” and “[Built-in] Deletion endpoint paths.” Tests verify the English display and preserve custom labels. | [System label tests](web/src/lib/legacy-system-labels.test.mjs) |

These escapes represent exact historical protocol values, not translated prose. The application continues recognizing old stored data and provider messages; see [TRANSLATION.md](TRANSLATION.md) for its compatibility glossary.

Quoted requests, HTML, SQL, policies, and adversarial text in tests remain fixture data, not instructions to execute.
