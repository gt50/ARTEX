import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

const source = await readFile(new URL("./auth-event-source.ts", import.meta.url), "utf8");
const compiled = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } });
const { AuthEventSource } = await import(`data:text/javascript;base64,${Buffer.from(compiled.outputText).toString("base64")}`);
const originalFetch = globalThis.fetch;
globalThis.localStorage = { getItem: () => "test-session" };

test("header-authenticated SSE parses split UTF-8, CRLF, named and multiline events", async () => {
  const bytes = new TextEncoder().encode("id: 7\r\nevent: snapshot\r\ndata: 世界\r\ndata: second\r\n\r\ndata: default\n\n");
  let request;
  globalThis.fetch = async (url, init) => {
    request = { url, ...init };
    return new Response(new ReadableStream({ start(controller) {
      for (const byte of bytes) controller.enqueue(Uint8Array.of(byte));
      controller.close();
    } }), { headers: { "Content-Type": "text/event-stream" } });
  };
  const events = [];
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("stream timed out")), 2000);
    const stream = new AuthEventSource("http://localhost/api/events");
    stream.addEventListener("snapshot", event => events.push([event.type,event.data,event.lastEventId]));
    stream.onmessage = event => { events.push([event.type,event.data,event.lastEventId]); stream.close(); clearTimeout(timer); resolve(); };
  });
  assert.deepEqual(events, [["snapshot","世界\nsecond","7"],["message","default","7"]]);
  assert.equal(request.headers.Authorization,"Bearer test-session");
  assert.equal(request.credentials,"omit");
  assert.ok(!request.url.includes("test-session"));
  globalThis.fetch = originalFetch;
});

test("reconnect retains the last complete cursor and stops on authorization failure", async () => {
  const requests = [];
  globalThis.fetch = async (_url, init) => {
    requests.push(init);
    if (requests.length === 1) return new Response("retry: 1\nid: cursor-12\ndata: first\n\nid: interrupted-13\n", { headers: { "Content-Type":"text/event-stream" } });
    return new Response("unauthorized", { status:401 });
  };
  await new Promise((resolve,reject) => {
    const timer = setTimeout(() => reject(new Error("reconnection timed out")), 3000);
    const stream = new AuthEventSource("http://localhost/api/events");
    stream.onerror = () => { if (requests.length === 2) { clearTimeout(timer); resolve(); } };
  });
  assert.equal(requests.length,2);
  assert.equal(requests[1].headers["Last-Event-ID"],"cursor-12");
  globalThis.fetch = originalFetch;
});

test("ID-only frames reset the cursor and terminal CR completes a frame", async () => {
  globalThis.fetch = async () => new Response("id: old\n\ndata: first\n\nid:\n\ndata: second\r\r", { headers: { "Content-Type":"text/event-stream" } });
  const events = [];
  await new Promise((resolve,reject) => {
    const stream = new AuthEventSource("http://localhost/api/events");
    const timer = setTimeout(() => { stream.close(); reject(new Error("terminal CR event timed out")); },2000);
    stream.onmessage = event => {
      events.push([event.data,event.lastEventId]);
      if (events.length === 2) { stream.close(); clearTimeout(timer); resolve(); }
    };
  });
  assert.deepEqual(events,[["first","old"],["second",""]]);
  globalThis.fetch = originalFetch;
});
