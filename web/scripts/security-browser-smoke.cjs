// Run inside the disposable candidate with the self-signed TLS fixture on the
// artex_default network. The fixture must respond with "forwarded-tls".
const fs = require("node:fs");
const { chromium } = require("/usr/lib/node_modules/playwright");

(async () => {
  const config = JSON.parse(fs.readFileSync("/app/auth/ca/browser-mcp.json", "utf8"));
  const target = "https://artex-security-target:19998/tls-probe";
  const captured = await chromium.launch({ ...config.browser.launchOptions, headless: true });
  try {
    const context = await captured.newContext({ ignoreHTTPSErrors: false });
    const page = await context.newPage();
    const response = await page.goto(target);
    if (response.status() !== 200 || await page.textContent("body") !== "forwarded-tls") {
      throw new Error("Authenticated HTTPS capture failed");
    }
    console.log("PASS HTTPS capture with certificate verification enabled");
  } finally {
    await captured.close();
  }

  const direct = await chromium.launch({ headless: true });
  try {
    const page = await direct.newPage({ ignoreHTTPSErrors: false });
    let rejected = false;
    try {
      await page.goto(target);
    } catch (error) {
      rejected = error.message.includes("ERR_CERT_AUTHORITY_INVALID");
    }
    if (!rejected) throw new Error("Untrusted direct certificate was accepted");
    console.log("PASS untrusted direct TLS still rejected");
  } finally {
    await direct.close();
  }
})().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
