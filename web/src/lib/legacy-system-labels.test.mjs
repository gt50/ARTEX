import assert from "node:assert/strict";
import test from "node:test";
import { englishSystemLabel } from "./legacy-system-labels.mjs";

const legacyDeletionRuleTitle = "[\u5185\u7f6e] \u5220\u9664\u7c7b\u63a5\u53e3\u8def\u5f84"; // [Built-in] Deletion endpoint paths
const legacyConversationTitle = "\u65b0\u5bf9\u8bdd"; // New conversation

test("legacy system labels display in English", () => {
  assert.equal(englishSystemLabel(legacyConversationTitle), "New conversation");
  assert.equal(englishSystemLabel(legacyDeletionRuleTitle), "[Built-in] Deletion endpoint paths");
});

test("user-authored labels and English labels are preserved", () => {
  for (const label of ["My conversation", `${legacyConversationTitle} about a custom task`, "", "[Built-in] Deletion endpoint paths"]) {
    assert.equal(englishSystemLabel(label), label);
  }
});
