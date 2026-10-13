import assert from "node:assert/strict";
import test from "node:test";
import { englishSystemLabel } from "./legacy-system-labels.mjs";

test("legacy system labels display in English", () => {
  assert.equal(englishSystemLabel("新对话"), "New conversation");
  assert.equal(englishSystemLabel("[内置] 删除类接口路径"), "[Built-in] Deletion endpoint paths");
});

test("user-authored labels and English labels are preserved", () => {
  for (const label of ["My conversation", "新对话 about a custom task", "", "[Built-in] Deletion endpoint paths"]) {
    assert.equal(englishSystemLabel(label), label);
  }
});
