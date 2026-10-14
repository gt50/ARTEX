// Fetch-based SSE keeps bearer credentials out of URLs, logs and history.
export class AuthEventSource {
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  private listeners = new Map<string, Set<(event: MessageEvent<string>) => void>>();
  private controller = new AbortController();
  private closed = false;
  private lastID = "";
  private retry = 3000;
  private timer: ReturnType<typeof setTimeout> | undefined;

  constructor(private url: string) { void this.connect(); }

  addEventListener(type: string, listener: (event: MessageEvent<string>) => void) {
    const listeners = this.listeners.get(type) ?? new Set();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }

  close() {
    this.closed = true;
    this.controller.abort();
    clearTimeout(this.timer);
  }

  private async connect() {
    try {
      const token = localStorage.getItem("artex_token");
      const response = await fetch(this.url, {
        headers: {
          Accept: "text/event-stream",
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
          ...(this.lastID ? { "Last-Event-ID": this.lastID } : {}),
        },
        signal: this.controller.signal,
        cache: "no-store",
        credentials: "omit",
      });
      if (response.status === 204) { this.close(); return; }
      if (response.status === 401 || response.status === 403) {
        this.onerror?.(new Event("error"));
        this.close();
        return;
      }
      if (!response.ok || !response.body || !response.headers.get("content-type")?.startsWith("text/event-stream")) {
        throw new Error("SSE connection failed");
      }
      this.onopen?.(new Event("open"));
      const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
      let buffer = "";
      let data: string[] = [];
      let type = "message";
      let pendingID = "";
      const line = (value: string) => {
        if (!value) {
          // Commit only at a frame boundary so interrupted events can replay.
          this.lastID = pendingID;
          if (data.length) {
            const event = new MessageEvent(type, { data: data.join("\n"), lastEventId: this.lastID });
            if (type === "message") this.onmessage?.(event);
            this.listeners.get(type)?.forEach((listener) => listener(event));
          }
          data = [];
          type = "message";
          return;
        }
        const colon = value.indexOf(":");
        const field = colon < 0 ? value : value.slice(0, colon);
        const content = colon < 0 ? "" : value.slice(colon + 1).replace(/^ /, "");
        if (field === "data") data.push(content);
        if (field === "event") type = content || "message";
        if (field === "id" && !content.includes("\0")) pendingID = content;
        if (field === "retry" && /^\d+$/.test(content)) this.retry = Math.min(60000, Math.max(1000, Number(content)));
      };
      while (!this.closed) {
        const chunk = await reader.read();
        if (chunk.done) {
          if (buffer.endsWith("\r")) line(buffer.slice(0, -1));
          break;
        }
        buffer += chunk.value;
        // Preserve trailing CR until the next chunk so split CRLF is one newline.
        let match: RegExpExecArray | null;
        while ((match = /\r\n|\r(?!$)|\n/.exec(buffer))) {
          line(buffer.slice(0, match.index));
          buffer = buffer.slice(match.index + match[0].length);
          if (this.closed) break;
        }
      }
      await reader.cancel();
      if (!this.closed) throw new Error("SSE disconnected");
    } catch {
      if (!this.closed) {
        this.onerror?.(new Event("error"));
        if (!this.closed) this.timer = setTimeout(() => { if (!this.closed) void this.connect(); }, this.retry);
      }
    }
  }
}
