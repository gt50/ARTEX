"use client";

//LLM Shared retry configuration: "number of times + interval" for each of the five layers of retries.
//
//Five layers from inside to outside: connection establishment (SDK) → empty response (SDK) → same provider security window → polling circuit breaker → intention to rerun.
//The first three layers follow the endpoints, so each model configuration can override the global default; the last two layers are process-level and only have a global copy.
//
//All inputs follow the same set of "leave blank = not configured" semantics, consistent with the backend db.RetryRule:
//Number of times Null/0 = Use the built-in default | -1 = Turn off this layer and try again | >0 = Use this number of times
//Interval empty/0 = use this layer's original exponential backoff | >0 = use this fixed millisecond interval instead

import * as React from "react";

import { Loader2Icon, SaveIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import type { LLMRetryOverride, LLMRetryPolicy, LLMRetryRule } from "@/lib/types";

export const ZERO_RULE: LLMRetryRule = { attempts: 0, interval_ms: 0 };
export const ZERO_OVERRIDE: LLMRetryOverride = {
  connect: ZERO_RULE,
  empty: ZERO_RULE,
  stream: ZERO_RULE,
};
const ZERO_POLICY: LLMRetryPolicy = {
  ...ZERO_OVERRIDE,
  breaker: ZERO_RULE,
  intent: ZERO_RULE,
};

type LayerMeta = {
  title: string;
  /** Where does this layer of retry occur and who performs it */
  where: string;
  /** What kind of errors will reach this level - specific to the status code, don't let anyone guess */
  trigger: string;
  /** It looks similar but [does] not go to this level, so as not to think it is a bug when there is no response after filling in the form */
  skips?: string;
  desc: string;
  attemptsLabel: string;
  /** The default value when the number of times is left blank, used for placeholders */
  defAttempts: number;
  /** Default strategy when interval is left empty, used for placeholders */
  defInterval: string;
  /** The meaning of filling in -1 for the number of times */
  offHint: string;
};

export const RETRY_LAYERS = {
  connect: {
    title: "Connection retry",
    where: "SDK · Before receiving HTTP 200",
    trigger:
      "Cannot connect or receive HTTP 200: network errors such as connection resets, read/write timeouts, and DNS failures, plus HTTP 408, 429, 500, 502, 503, and 504.",
    skips: "Other status codes (400 / 401 / 403 / 404 / 413 / 422, etc.) are definitive rejections. Retrying would fail again, so the error is returned immediately.",
    desc: "Resends the same request unchanged. Once a stream starts (HTTP 200 received), interruptions are handled by another layer.",
    attemptsLabel: "Retry attempts",
    defAttempts: 3,
    defInterval: "Exponential: 0.5s → 1s → 2s (max 8s)",
    offHint: "-1 = do not retry; return the error immediately",
  },
  empty: {
    title: "Empty response retry",
    where: "SDK · OpenAI format only",
    trigger:
      "The response has HTTP 200 and a normal stop finish_reason, but contains no content blocks. This can happen with empty gateway frames, dropped reasoning fields, or sampling glitches.",
    skips: "Responses with no content because max_tokens was reached are excluded. Increase the output limit; retrying would hit the same limit again.",
    desc: "Resends the full prompt, which can be expensive for long contexts. Keep the retry count low.",
    attemptsLabel: "Retry attempts",
    defAttempts: 2,
    defInterval: "Exponential: 0.5s → 1s → 2s (max 8s)",
    offHint: "-1 = return the empty response unchanged",
  },
  stream: {
    title: "Same-provider safe retry",
    where: "Application · Before any output is delivered",
    trigger:
      "An error occurs after the stream is established (HTTP 200 received): connection drops, provider overload, or an in-stream 429 / 5xx error, before any tokens are delivered to the caller.",
    skips:
      "Quota exhaustion (402 / insufficient_quota) is handled by failover; oversized context (413 / context length) is handled by compaction. Definitive rejections (400 / 401 / 403 / 404 / 422) are not retried.",
    desc: "Replays the request with the same profile. Since no output was delivered, this cannot duplicate model output or tool execution.",
    attemptsLabel: "Retry attempts",
    defAttempts: 2,
    defInterval: "Exponential: 0.5s → 1s (max 4s)",
    offHint: "-1 = pass stream failures to the outer intent retry",
  },
  breaker: {
    title: "Profile circuit breaker",
    where: "Application · Process-wide, one global policy",
    trigger:
      "The breaker opens after repeated transient failures (429, 5xx, network errors) reach the threshold. Definitive failures such as insufficient balance (402), invalid keys (401 / 403), and missing models (404) open it immediately.",
    skips: "A successful request resets the count, so occasional failures do not accumulate toward the threshold.",
    desc: "When open, the profile enters a cooldown and failover skips it. State is stored in the database and survives restarts.",
    attemptsLabel: "Consecutive failures before opening",
    defAttempts: 3,
    defInterval: "Progressive: 1 min → 5 min → 30 min",
    offHint: "-1 = never open on transient failures (definitive failures still open it)",
  },
  intent: {
    title: "Intent retry",
    where: "Application · Process-wide, one global policy",
    trigger:
      "The earlier layers could not recover: the worker ended with model_error because inner retries were exhausted, or the stream failed after output began (replaying is unsafe, so the whole intent must restart).",
    skips: "Quota exhaustion is handled by profile failover. Retries yield immediately when a task is paused, stopped, or finishing, without consuming the backoff delay.",
    desc: "Restarts the entire intent. This is the outermost layer, so each retry multiplies the attempts in the inner layers.",
    attemptsLabel: "Intent retries",
    defAttempts: 2,
    defInterval: "Fixed: 3s",
    offHint: "-1 = do not retry; mark the intent as blocked",
  },
} satisfies Record<string, LayerMeta>;

type LayerKey = keyof typeof RETRY_LAYERS;

/** Human words in milliseconds, only used to echo next to the input box to avoid counting zeros. */
function humanMs(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) return "";
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${Number((ms / 1000).toFixed(2))}s`;
  return `${Number((ms / 60_000).toFixed(2))}min`;
}

/** Controlled digital input: empty string ↔ 0, intermediate states ("-", "1e") remain local as they are, without disturbing the parent. */
function NumField({
  id,
  value,
  onChange,
  placeholder,
  min,
}: {
  id: string;
  value: number;
  onChange: (n: number) => void;
  placeholder: string;
  min: number;
}) {
  const [text, setText] = React.useState(value === 0 ? "" : String(value));
  //It will keep up when the parent changes a complete set of values ​​(reading policies, switching configurations); it will not go here when typing by yourself.
  //Because at that time value is already equal to the result of local text parse.
  React.useEffect(() => {
    const incoming = value === 0 ? "" : String(value);
    setText((cur) => (Number(cur || 0) === value ? cur : incoming));
  }, [value]);
  return (
    <Input
      id={id}
      type="number"
      min={min}
      className="w-28 shrink-0"
      value={text}
      placeholder={placeholder}
      onChange={(e) => {
        setText(e.target.value);
        const n = Number(e.target.value);
        onChange(e.target.value.trim() === "" || !Number.isFinite(n) ? 0 : Math.trunc(n));
      }}
    />
  );
}

/** Two knobs for one level retry. idPrefix is ​​used to preserve the label's htmlFor when the same page appears multiple times. */
export function RetryRuleFields({
  layer,
  idPrefix,
  value,
  onChange,
  compact,
}: {
  layer: LayerKey;
  idPrefix: string;
  value: LLMRetryRule;
  onChange: (r: LLMRetryRule) => void;
  /** true = compact version of the configuration drawer: omit the expansion description, leaving only the sentence "What error will reach this level" */
  compact?: boolean;
}) {
  const meta = RETRY_LAYERS[layer];
  const human = humanMs(value.interval_ms);
  return (
    <div className={compact ? "grid gap-2" : "grid gap-3 rounded-lg border p-3"}>
      <div className="grid gap-0.5">
        <div className="flex flex-wrap items-baseline gap-2">
          <Label className="text-sm">{meta.title}</Label>
          <span className="text-muted-foreground text-xs">{meta.where}</span>
        </div>
        {/*Which errors will reach this level, specifically the status code - if you fill in the knob but can't see the effect, it's probably because the error doesn't fall on this level at all.*/}
        <p className="text-muted-foreground text-xs">
          <span className="font-medium text-foreground">When it retries:</span> {meta.trigger}
        </p>
        {!compact && meta.skips && (
          <p className="text-muted-foreground text-xs">
            <span className="font-medium text-foreground">Not handled here:</span> {meta.skips}
          </p>
        )}
        {!compact && <p className="text-muted-foreground text-xs">{meta.desc}</p>}
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="flex items-center gap-2">
          <Label htmlFor={`${idPrefix}-${layer}-n`} className="text-muted-foreground text-xs">
            {meta.attemptsLabel}
          </Label>
          <NumField
            id={`${idPrefix}-${layer}-n`}
            min={-1}
            value={value.attempts}
            placeholder={`Default: ${meta.defAttempts}`}
            onChange={(n) => onChange({ ...value, attempts: n })}
          />
        </div>
        <div className="flex items-center gap-2">
          <Label htmlFor={`${idPrefix}-${layer}-ms`} className="text-muted-foreground text-xs">
            Interval (ms)
          </Label>
          <NumField
            id={`${idPrefix}-${layer}-ms`}
            min={0}
            value={value.interval_ms}
            placeholder="Default backoff"
            onChange={(n) => onChange({ ...value, interval_ms: n })}
          />
          <span className="text-muted-foreground text-xs">{human ? `Fixed: ${human}` : meta.defInterval}</span>
        </div>
      </div>
      {!compact && <p className="text-muted-foreground text-xs">Blank = use default; {meta.offHint}.</p>}
    </div>
  );
}

/** Three layers of coverage in the model configuration drawer (the three layers that follow the endpoints). */
export function ProfileRetryFields({
  value,
  onChange,
}: {
  value: LLMRetryOverride;
  onChange: (o: LLMRetryOverride) => void;
}) {
  return (
    <div className="grid gap-3 rounded-lg border p-3">
      <div className="grid gap-0.5">
        <Label className="text-sm">Retry overrides</Label>
        <p className="text-muted-foreground text-xs">
          Applies only to this profile and overrides the global defaults in “Retries and backoff.” Leave a field blank to use the global value; set attempts to -1 to disable that retry layer;
          set an interval to replace exponential backoff with a fixed delay. The circuit breaker and intent retry are process-wide and can only be configured in global settings.
        </p>
      </div>
      {(["connect", "empty", "stream"] as const).map((k) => (
        <div key={k} className="border-t pt-3 first:border-t-0 first:pt-0">
          <RetryRuleFields
            compact
            layer={k}
            idPrefix="pf"
            value={value[k]}
            onChange={(r) => onChange({ ...value, [k]: r })}
          />
        </div>
      ))}
    </div>
  );
}

/** "Retry and Backoff" tab: Global default value for layer five. */
export function RetryPolicyPanel() {
  const [policy, setPolicy] = React.useState<LLMRetryPolicy>(ZERO_POLICY);
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const p = await api.llmRetryPolicy();
      setPolicy({ ...ZERO_POLICY, ...p });
    } catch (e) {
      toast.error(`Could not load retry policy: ${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  async function save() {
    if (saving) return;
    setSaving(true);
    try {
      //The backend will clamp the out-of-bounds value back into the range and return it, directly refreshing it with the returned value. What you see is what you save.
      const saved = await api.saveLLMRetryPolicy(policy);
      setPolicy({ ...ZERO_POLICY, ...saved });
      toast.success("Saved and applied immediately (the current request continues with the previous settings)");
    } catch (e) {
      toast.error(`Save failed: ${(e as Error).message}`);
    } finally {
      setSaving(false);
    }
  }

  const set = (k: LayerKey) => (r: LLMRetryRule) => setPolicy((p) => ({ ...p, [k]: r }));

  if (loading) {
    return (
      <div className="flex items-center gap-2 rounded-lg border border-dashed p-10 text-muted-foreground text-sm">
        <Loader2Icon className="size-4 animate-spin" /> Loading retry policy…
      </div>
    );
  }

  return (
    <div className="grid gap-4">
      <div className="rounded-lg border bg-muted/30 p-3 text-muted-foreground text-xs leading-relaxed">
        Failed model requests pass through five retry layers, from inner to outer:
        <span className="text-foreground"> connection → empty response → same-provider safe retry → profile circuit breaker → intent retry</span>
        . Outer layers run only after inner retries are exhausted, so the attempt counts
        <span className="text-foreground"> multiply</span>
        ; setting every layer to its maximum could turn one transient error into dozens of requests.
        Leave all fields blank to keep the current defaults. The first three layers can be overridden per model profile.
      </div>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {(Object.keys(RETRY_LAYERS) as LayerKey[]).map((k) => (
          <RetryRuleFields key={k} layer={k} idPrefix="gl" value={policy[k]} onChange={set(k)} />
        ))}
      </div>

      <div className="flex gap-2">
        <Button onClick={save} disabled={saving}>
          {saving ? <Loader2Icon className="animate-spin" /> : <SaveIcon />}
          Save
        </Button>
        <Button variant="outline" onClick={() => setPolicy(ZERO_POLICY)} disabled={saving}>
          Restore all defaults
        </Button>
      </div>
    </div>
  );
}
