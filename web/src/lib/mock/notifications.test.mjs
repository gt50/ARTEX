import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, resolve } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import ts from "typescript";

// Load the actual mock router and its local TypeScript dependencies. Type-only
// imports disappear during compilation; no server or generated files are needed.
const cache = new Map();
const require = createRequire(import.meta.url);
function loadTS(path) {
  if (cache.has(path)) return cache.get(path).exports;
  const module = { exports: {} };
  cache.set(path, module);
  const code = ts.transpileModule(readFileSync(path, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  const localRequire = (specifier) =>
    specifier.startsWith(".") ? loadTS(resolve(dirname(path), `${specifier}.ts`)) : require(specifier);
  new Function("require", "module", "exports", code)(localRequire, module, module.exports);
  return module.exports;
}
const { mockHandle } = loadTS(fileURLToPath(new URL("./handler.ts", import.meta.url)));

test("notification demo metadata and lists satisfy the page contract", async () => {
  const meta = await mockHandle("GET", "/notify/meta");
  assert.deepEqual(meta.kinds.map((kind) => kind.kind), ["dingtalk", "email", "feishu", "telegram", "webhook", "wecom"]);
  assert.equal(meta.enabled, false);
  assert.equal(meta.defaults.digest_interval_min, 30);
  assert.equal(meta.stats.channels, 0);
  assert.deepEqual(await mockHandle("GET", "/notify/channels"), { channels: [] });
  assert.deepEqual(await mockHandle("GET", "/notify/deliveries?page=1&page_size=50"), { deliveries: [], total: 0 });
  meta.kinds.length = 0;
  assert.equal((await mockHandle("GET", "/notify/meta")).kinds.length, 6);
});

test("notification demo does not claim to edit channels or send messages", async () => {
  for (const [method, path] of [["POST", "/notify/channels"], ["PATCH", "/notify/channels/1"], ["POST", "/notify/channels/1/test"]]) {
    await assert.rejects(mockHandle(method, path), /demo mode is read-only/);
  }
});
