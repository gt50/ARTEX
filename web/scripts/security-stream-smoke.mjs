// Against the disposable smoke deployment only; never pass production credentials.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import ts from "typescript";

const base = process.env.ARTEX_SMOKE_BASE ?? "http://127.0.0.1:18787";
const secretsPath = process.env.ARTEX_SMOKE_SECRETS;
if (!secretsPath) throw new Error("ARTEX_SMOKE_SECRETS must name the disposable test credential file");
const secrets = JSON.parse(await readFile(secretsPath,"utf8"));
let token = secrets.token;
if (!token) {
const response = await fetch(`${base}/api/auth/login`, {
  method:"POST", headers:{"Content-Type":"application/json"},
  body:JSON.stringify({username:secrets.username ?? "ARTEX",password:secrets.admin}),
});
assert.equal(response.status,200,"disposable login failed");
({token} = await response.json());
}
globalThis.localStorage = {getItem:()=>token};
const source = await readFile(new URL("../src/lib/auth-event-source.ts",import.meta.url),"utf8");
const compiled = ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ES2022}});
const {AuthEventSource} = await import(`data:text/javascript;base64,${Buffer.from(compiled.outputText).toString("base64")}`);
await new Promise((resolve,reject)=>{
  const stream = new AuthEventSource(`${base}/api/logs/stream?since=0`);
  const timer = setTimeout(()=>{stream.close();reject(new Error("authenticated stream received no event"));},10000);
  stream.onmessage = event=>{
    assert.ok(event.data.length>0);
    stream.close(); clearTimeout(timer); resolve();
  };
  stream.onerror = ()=>{stream.close();clearTimeout(timer);reject(new Error("authenticated stream failed"));};
});
console.log("PASS application SSE client receives live authenticated log events without URL credentials");
