package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
)

// Planner is the event-driven LLM planner (docs §4.3): each time the asset or
// exploration graph changes (debounced), it reads the exploration route, queries
// assets, judges whether the task goal is met, and emits 0..N exploration intents
// into the frontier. It is the sole intent generator.
type Planner struct {
	findingRecorder   FindingRecorder
	prov              llm.Provider
	model             string
	tx                *transcript.Store                      // raw LLM conversation persistence (nil = off)
	window            int                                    // context window in tokens (for compaction)
	windowFn          func() int                             // optional dynamic task-chain minimum
	maxTurns          int                                    // max agent turns per run (0 = unlimited)
	killWork          func(intentID int64) error             // engine callback to terminate a running work (nil = off)
	steerWork         func(intentID int64, msg string) error // engine callback to steer a running work mid-run (nil = off)
	proxyAddr         string                                 // recording proxy for WebFetch (empty = direct)
	proxyCACert       string                                 // recording proxy's CA cert path (HTTPS verify)
	webSearch         WebSearchOpts                          // web_search tool backend selection (off by default)
	workDir           string                                 // shared work dir (surfaced in prompt as artifact-output target)
	injectConstraints func() bool                            // resolver: inject task operation constraints into system prompt? (nil = yes)
	nonStreamingFn    func() bool                            // resolver: use non-streaming (Complete) path? (nil = streaming)
	noaEnabledFn      func() bool                            // resolver: use experimental noa compaction? (nil = off)
	maxTokensFn       func() int                             // resolver: per-reply output cap (nil/0 = send no cap)
	compactor         *Compactor                             // cold-node compaction (§7); nil = disabled

	// todos keeps ONE plan-scratchpad per task (keyed by exploration id) so the
	// planner's multi-step plan survives across wake-ups — each Plan() is a fresh
	// session, but the shared store lets it record a serial exploit chain once and
	// dispatch it step-by-step over rounds instead of front-loading it in parallel.
	todoMu sync.Mutex
	todos  map[int64]*actool.TodoStore
}

func NewPlanner(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int) *Planner {
	return &Planner{prov: prov, model: model, workDir: workDir, tx: tx, window: window, maxTurns: maxTurns, todos: map[int64]*actool.TodoStore{}}
}

func (p *Planner) SetCompactionWindowResolver(fn func() int) { p.windowFn = fn }

// SetCompactor wires the cold-node compactor (cold-digest §7). Called each
// planner wake-up to advance the round counter, maintain cold stamps, and
// (off the hot path) fold cold nodes into digests. nil = feature disabled.
func (p *Planner) SetCompactor(c *Compactor) { p.compactor = c }

// SetNonStreaming wires a resolver deciding whether runs use the non-streaming
// model path (true = non-streaming). nil/unset = streaming (default).
func (p *Planner) SetNonStreaming(fn func() bool) { p.nonStreamingFn = fn }

func (p *Planner) nonStreaming() bool { return p.nonStreamingFn != nil && p.nonStreamingFn() }

// SetNoaEnabled wires a resolver deciding whether runs use the experimental noa
// context-compression mechanism. nil/unset = off (built-in compaction). Read per
// run so the settings toggle takes effect without rebuilding the agent.
func (p *Planner) SetNoaEnabled(fn func() bool) { p.noaEnabledFn = fn }

// SetMaxTokens wires a resolver for the per-reply output cap. nil/unset or 0 =
// send no cap and let the endpoint decide. Read per run, like nonStreaming.
func (p *Planner) SetMaxTokens(fn func() int) { p.maxTokensFn = fn }

func (p *Planner) maxTokens() int {
	if p.maxTokensFn == nil {
		return 0
	}
	return p.maxTokensFn()
}

func (p *Planner) compactionWindow() int {
	if p.windowFn != nil {
		return p.windowFn()
	}
	return p.window
}

// SetProxy points the planner's WebFetch at the recording proxy plus the CA cert
// it trusts to verify HTTPS through it (empty addr = direct).
func (p *Planner) SetProxy(addr, caCert string) { p.proxyAddr, p.proxyCACert = addr, caCert }

// SetWebSearch selects the web_search backend for the planner (off by default).
func (p *Planner) SetWebSearch(o WebSearchOpts) { p.webSearch = o }

// SetConstraintInject wires a resolver deciding whether this task's operation
// constraints get injected into the planner system prompt. Read per round so the
// settings toggle takes effect without rebuilding the agent. nil = inject (default).
func (p *Planner) SetConstraintInject(fn func() bool) { p.injectConstraints = fn }

// wantConstraints reports whether constraint injection is enabled (default yes).
func (p *Planner) wantConstraints() bool { return p.injectConstraints == nil || p.injectConstraints() }

// todoFor returns the task's persistent planning todo store, creating it on first
// use. Shared across all of this task's planner wake-ups.
func (p *Planner) todoFor(expID int64) *actool.TodoStore {
	p.todoMu.Lock()
	defer p.todoMu.Unlock()
	s := p.todos[expID]
	if s == nil {
		s = actool.NewTodoStore()
		p.todos[expID] = s
	}
	return s
}

// SetKillWork wires the engine's per-work terminate callback so the planner's
// kill_work tool can stop a single running worker.
func (p *Planner) SetKillWork(fn func(intentID int64) error) { p.killWork = fn }

// SetSteerWork wires the engine's per-work steering callback so the planner's
// steer_work tool can inject a mid-run course-correction into a running worker.
func (p *Planner) SetSteerWork(fn func(intentID int64, msg string) error) { p.steerWork = fn }

// renderPlannerTodos formats the persistent planning todo for injection into the
// wake-up prompt (empty when there are no todos yet — first wake-up).
func renderPlannerTodos(items []actool.Todo) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[Your planning todo list, persisted across wake-ups and written by you last round]:\n")
	for _, it := range items {
		mark := map[actool.TodoStatus]string{actool.TodoPending: "☐", actool.TodoInProgress: "▶", actool.TodoCompleted: "✔"}[it.Status]
		if mark == "" {
			mark = "☐"
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", mark, it.Content))
	}
	b.WriteString("Continue from this list: dispatch an intent only for the next step whose prerequisites are complete or whose required fact exists. Use TodoWrite to update the list, marking steps satisfied by facts as completed. Do not dispatch steps already pending or in progress.")
	return b.String()
}

// TriggerEvent describes what concretely caused this planning round to fire, so
// the planner looks first at the actual change instead of re-scanning the whole
// overview. Kind:
//
//	"done"    — a worker finished intent IntentID (its output conclusion is fetched).
//"finding" — a worker reported a finding on intent IntentID (Detail = summary).
//"goal" — the human (via main agent's set_goals) added one OR MORE goals in a
//single call (Goals = the new target text this time, 1+ items; set_goals supports batches).
//"goal_deleted" — the human deleted a goal from overview goal management (Detail = deleted goal text).
//"goal_edited" — the human edited a goal from Overview of goal management (OldGoal→NewGoal text).
//"cancelled" — the human deleted intent IntentID (Detail = reason for deletion). The intent is
//	            stopped (not deleted) and the reason is attached to it as a fact.
type TriggerEvent struct {
	Kind     string
	IntentID int64
	Detail   string
	Summary  string   //Kind=="cancelled" Exclusive: Summary of intent captured before deletion (the node no longer exists after deletion and cannot be checked again)
	Goals    []string //Kind=="goal" Exclusive: the target text (1 or more) added by set_goals this time
	OldGoal  string   //Kind=="goal_edited" Special: target text before modification
	NewGoal  string   //Kind=="goal_edited" Exclusive: Modified goal text
	Hints    []string //Kind=="hint" exclusive: the new hint text (1 or more) added by this add_hint
}

// renderTriggers spells out the change(s) that fired this round: for a finished
// worker — which intent + its output conclusion; for a finding — which intent +
// what was found. Empty for time/heartbeat wakes. Reads the store (best-effort;
// a blank field never blocks the round).
func renderTriggers(ts *db.ExplorationStore, evs []TriggerEvent) string {
	if len(evs) == 0 || ts == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[Changes that triggered this planning round (review these first, then decide whether to add directions)]:")
	for _, ev := range evs {
		switch ev.Kind {
		case "goal":
			if len(ev.Goals) == 1 {
				b.WriteString(fmt.Sprintf("\n- The user (through the main agent) added a goal: %s — this is a new outcome to achieve. Add exploration directions if no corresponding intent exists.", ev.Goals[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- The user (through the main agent) added %d goals: %s — each is a new outcome to achieve. Add exploration directions for goals that do not yet have a corresponding intent.", len(ev.Goals), strings.Join(ev.Goals, "; ")))
			}
		case "hint":
			if len(ev.Hints) == 1 {
				b.WriteString(fmt.Sprintf("\n- The user (through the main agent) added a strategic hint: %s — it is in the exploration graph. Adjust or add exploration directions if no corresponding intent exists.", ev.Hints[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- The user (through the main agent) added %d strategic hints: %s — all are in the exploration graph. Adjust or add exploration directions for each.", len(ev.Hints), strings.Join(ev.Hints, "; ")))
			}
		case "goal_deleted":
			b.WriteString(fmt.Sprintf("\n- The user deleted this goal: %s — it has been removed. Reassess the remaining goals and directions; do not dispatch more intents for it.", ev.Detail))
		case "goal_edited":
			b.WriteString(fmt.Sprintf("\n- The user changed a goal from '%s' to '%s'. Adjust exploration directions for the new goal and stop dispatching any that no longer apply.", ev.OldGoal, ev.NewGoal))
		case "finding":
			b.WriteString(fmt.Sprintf("\n- The Worker for intent #%d (%s) reported a finding: %s", ev.IntentID, intentSummary(ts, ev.IntentID), ev.Detail))
		case "cancelled":
			//The intent content is given priority by using the summary captured during deletion (the node no longer exists after hard deletion, and intentSummary cannot be found).
			sm := ev.Summary
			if sm == "" {
				sm = intentSummary(ts, ev.IntentID)
			}
			b.WriteString(fmt.Sprintf("\n- The user deleted intent #%d: %s. Reason: %s. This intent will not run; replan accordingly.", ev.IntentID, sm, ev.Detail))
		default: // "done"
			b.WriteString(fmt.Sprintf("\n- The Worker for intent #%d (%s) finished. Conclusion: %s", ev.IntentID, intentSummary(ts, ev.IntentID), workerOutput(ts, ev.IntentID)))
			if fids := factIDsYielded(ts, ev.IntentID); fids != "" {
				b.WriteString(fmt.Sprintf("; new fact IDs produced by this intent: %s ", fids))
			}
		}
	}
	b.WriteString("\n(Use node_detail, get_worker_output, or list_findings for full details.)")
	return b.String()
}

// factIDsYielded lists the fact ids an intent produced this run as "#12、#15", so the
// planner can jump straight to the round's incremental facts. Empty (best-effort) when
// the intent yielded no facts or the lookup fails.
func factIDsYielded(ts *db.ExplorationStore, id int64) string {
	ids, err := ts.FactsYielded(id)
	if err != nil || len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, fid := range ids {
		parts[i] = fmt.Sprintf("#%d", fid)
	}
	return strings.Join(parts, "、")
}

// intentSummary reads an intent node's one-line summary (best-effort, "?" on miss).
func intentSummary(ts *db.ExplorationStore, id int64) string {
	n, err := ts.GetNode(id)
	if err != nil || n == nil {
		return "?"
	}
	var p map[string]any
	if json.Unmarshal(n.Payload, &p) == nil {
		if s, ok := p["summary"].(string); ok && s != "" {
			return s
		}
	}
	return "?"
}

// workerOutput returns the finished worker's conclusion for an intent — the last
// 'result' (else 'text') activity's full detail, truncated. Same source get_worker_output uses.
func workerOutput(ts *db.ExplorationStore, id int64) string {
	acts, _, err := ts.ActivityList(&id, 0, 1000)
	if err != nil {
		return "(failed to retrieve output)"
	}
	var pick *db.Activity
	for i := range acts {
		if acts[i].Kind == "result" {
			pick = &acts[i]
		} else if acts[i].Kind == "text" && pick == nil {
			pick = &acts[i]
		}
	}
	if pick == nil {
		return "(no output recorded for this work yet)"
	}
	out, _ := ts.ActivityDetail(pick.ID)
	if out == "" {
		out = pick.Summary
	}
	return truncOutput(out, 800)
}

// truncOutput caps a worker-output blob so the trigger context doesn't bloat the
// system prompt every round; full text is one get_worker_output call away.
func truncOutput(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + " … (truncated; use get_worker_output for the full output)"
}

// renderGraphOverview folds the pre-computed graph_overview snapshot into the
// wake-up prompt so the planner starts each round with the full situation in
// hand — saving the round-trip it would otherwise spend calling the tool. It is
// the exact same JSON graph_overview would return; deeper detail is still one
// tool call away (node_detail / list_facts / …).
func renderGraphOverview(data map[string]any) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "" // fall back to the model calling graph_overview itself
	}
	return "\n\n[Situation for this round (prefetched graph_overview result; equivalent to calling the tool. Use node_detail/list_facts for more detail as needed)]:\n" + string(b)
}

//plannerDefaultTmpl is the built-in EDITABLE body (section [A]) of the planner prompt,
//seeded into agent_prompts. Goal is a {{.Goal}} template var; the intermediate product output specification
// tail is code-owned (artifactSpec) and appended by plannerSystem after rendering.
const plannerDefaultTmpl = `You are the planner in an authorized penetration-testing system. You are woken frequently, often whenever the graph changes. Your job is to review the situation, assess goals, and add exploration intents **only when there are genuinely new, uncovered directions**. You plan; you do not execute. This round's outputs must be limited to creating/clarifying intents or assessing goals. Never perform the work in the plan.

Task goal: {{.Goal}}

**Decide how many intents to produce this round:**
- **Hard requirement (highest priority):** If any goal remains unmet and there are no open or running intents (frontier_open=0 and running_intents is empty), you **must** produce at least one intent that advances a goal. With no running work to await and no queued direction, producing zero intents stalls the task. Even if all known directions appear only in recent_done, use the done/exhausted/blocked guidance below to create a new direction or resume one.
- Outside that hard requirement, producing zero intents is normal when there is a valid reason; do not default to "fewer is safer." Valid reasons include (1) **already covered:** every direction you considered is covered by an open/running intent (rephrasing an existing intent as a duplicate is a serious error); or (2) **waiting on a dependency:** the next step depends on output from work that is still running. Dispatching downstream work before its prerequisite exists would make it spin without useful input, so wait for the next wake-up after the graph updates.
- Conversely, dispatch when there is a genuinely uncovered direction that does not depend on running work, or when goals remain unmet and in-scope surfaces remain untested. Do not use zero intents as the lazy default.

**Decision process on each wake-up:**

1. **The complete situation is included below** (the graph_overview result; do not call graph_overview again): task (original title, goal, and root node), asset counts, goals and their states, open/running/recent_done intents, sites_without_endpoints (sites that may need exploration), facts (exploration facts, separate from findings), and recent_facts ({id, summary, optional confidence}).
   - **Scope:** Exploration nodes (goals/intents/facts/findings) belong only to this task. **The asset graph is shared globally** across tasks, and asset counts reflect all in-scope assets, not just assets for this task. Ignore assets unrelated to this task.
   - **Lineage:** Each intent has parents (upstream facts/intents it derives from) and yields (downstream facts/findings it produced); each recent_fact includes from_intent. Use these to understand which direction produced each fact and whether facts combine into a new direction.
   - **Negative or uncertain observations** (for example, "port closed" or "not injectable" in recent_facts) are Worker observations, not automatically final conclusions. Before accepting one, call node_detail(id) and inspect the evidence. Treat a direction as provisionally closed only when evidence is strong, confidence=observed, and the available methods were exhausted. If evidence is missing, the check was cursory ("looks like" or tested once), or confidence=inferred, treat the direction as **unexplored**. If in scope and not covered by another intent, normally dispatch one verification intent to confirm or refute it. **Verify each negative direction no more than once.** If the verification remains negative and the evidence is reasonable, accept the result and do not dispatch again.
   - **Fetch deeper detail only as needed:** list_facts (paginated, newest first, default 20; supports q, before, total, has_more), list_findings, node_detail(id) (full evidence/details; lists and recent_facts contain summaries only), list_assets (search/filter by q, type, company_id, task_id, paginate, or fetch by id/ids), and asset_neighbors. Assets are global; do not fetch everything by default.

2. **Assess goals (core responsibility):** The goals field includes each goal and its state. When a finding or fact proves an unmet goal, call prove_goal(goal_id, evidence_id, reason) to mark it met. **When you mark the last unmet goal as met, the system automatically marks the entire task complete.** Completion is driven by proving goals individually; there is no separate "complete task" action.
   - ⚠️ **Check measurable criteria; never mark a goal met early.** If a goal includes a measurable condition (for example, coverage of X%, N flags, or a specific privilege), **verify** the measured value in the graph_overview (such as coverage.pct or findings_total) before calling prove_goal. If the criterion is unmet, do not call prove_goal; dispatch intents to close the gap. Never mark a goal met because it is "mostly achieved" or the "main target is secured." Example: a 100% coverage goal with measured coverage.pct=40% is not met; continue testing.

3. **Optional, minimal orientation checks at startup only:** If the graph has almost no facts (recent_facts is empty and the task has just started) and the situation alone is insufficient to make initial intents specific, you may make a very small number of read-only checks with Bash (for example, one or two curl requests to inspect a home page or fingerprint). **The only valid output is a more precise intent description.** Do not discover, validate, or exploit vulnerabilities, and do not enumerate endpoints, directories, or parameters; those are Worker tasks to dispatch as intents.
   - If any Worker-produced facts already exist (facts>0 or recent_facts is nonempty), do not probe. Base all decisions on existing facts; this round may only dispatch intents or finish. To investigate a lead, dispatch an intent to a Worker; do not curl it yourself.
   - Even at startup, stop after at most three checks, solely to clarify initial intents. If you are investigating in depth instead of choosing a direction quickly (enumerating endpoints/directories, trying IDs one by one, decoding chains, repeatedly probing one endpoint, or testing injection/authorization/vulnerabilities), stop and turn the work into an intent immediately.
   - Do not probe when the existing facts and situation are enough to decide.

4. **Choose new directions:** **Restraint means avoiding duplicate intents, not dispatching as few as possible.** While goals remain unmet, ask what deeper, more effective, uncovered approaches could advance them, rather than whether you can wrap up. Intents are open-ended exploration directions, not a fixed menu. Infer directions from the facts, assets, and goals, then compare each with open, running, and recent_done intents:
   - If covered by an open/running intent, do not add it; it is already being handled.
   - If it appears in recent_done, **inspect that intent's state before deciding how to proceed:**
     - **done (completed normally):** It is covered; do not dispatch the same intent again. Whether it is a dead end depends on the facts it yielded, not the state alone. Retry only if there is a **materially new mechanism** (new fact, asset, parameter, or clearly different approach), and explain the difference in the summary. Rewording it or saying "try again in case it works" is not a new mechanism; do not retry.
     - **exhausted (budget ran out mid-exploration; partial results only) / blocked (model or network failure prevented meaningful exploration):** These ended prematurely and may have incomplete information. First inspect get_worker_trace / get_worker_output to see what was actually done and where it stopped. If it was close to a breakthrough but ran out of budget, dispatch an intent to continue from the last progress. If an external failure prevented the work (common for blocked), dispatch the same direction again. If the same obstacle recurs, change the approach or direction. Base this decision on actual trace progress, not the state alone.
   - Generate an intent for a genuinely new direction that no existing intent covers.
   - If all known directions are covered by open/running intents, produce none and finish; wait for that work to progress. But if only recent_done intents remain, with no open/running work and unmet goals, follow the hard requirement above and create or resume a direction.
   - **Depth before breadth:** Coverage is a floor and an acceptance measure, not the exploration goal itself. Once a high-value entry point is found (potential RCE, privilege escalation, or data exposure), prioritize pursuing that path deeply rather than shallowly testing every asset to even out coverage.
   - **Keep approaches diverse; do not converge too early:** While goals remain unmet, if current intents cluster around one path and a materially different uncovered approach exists (another entry point, asset class, or attack chain), prefer adding that distinct approach over another intent with the same mechanism. Judge substantive differences, not wording. If the distinct approach is already covered, do not add it. Ideally maintain two or three approaches with different mechanisms (for example, an upload path and an authentication-bypass path) until one provides evidence of goal progress; then focus resources there. **Operational constraints above always take precedence:** never add an intent for a surface, port, host, or action excluded by those constraints, even if it would diversify the approaches.

   **Dispatch sequential attack chains in steps, not in parallel.** For a strongly dependent chain (1 → 2 → 3, where each step requires the actual output of the previous one), do not dispatch all steps at once; downstream workers would lack prerequisites and repeat work or stall. Track the chain with TodoWrite, one step per item. Dispatch only the step whose prerequisites are already met (usually the first). After it produces a fact, dispatch the next step on a later wake-up and mark satisfied steps completed. Do not split one action into multiple intents ("confirm the trigger" and "trigger it" are one step). Parallelize only independent dimensions, such as enumerating unrelated endpoints.

5. **Submit:** Call add_intent **once** with the selected directions in an intents array (at most the four highest-value intents); do not submit them one by one.
   - **summary:** A natural-language sentence describing the direction, including the full target address, what to do, and why. Do not force it into a fixed category. Deduplication relies primarily on comparing summaries to existing intents.
   - **asset_ids:** IDs of assets to test/attack (zero, one, or several; obtain them from list_assets when possible). Always include them when the direction concerns specific assets such as a site, endpoint, parameter, or host. Include every relevant asset for a cross-asset direction. Leave empty only for global reconnaissance without a specific asset.
   - **parent_ids:** Optional upstream nodes that informed this direction (zero, one, or several). Include all facts combined to derive an intent, and include an upstream intent/finding when the direction follows from it. Leave empty for a genuinely new top-level direction.

Avoid duplicates and forced intents. But if goals remain unmet and a deeper uncovered approach exists, dispatch it. Be concise, focused, and efficient.`

func plannerSystem(goal, dataDir, workDir string) string {
	body := renderSystem("planner", plannerDefaultTmpl, PlannerVars{Goal: goal, DataDir: dataDir, Now: nowStr()})
	return body + artifactSpec(workDir)
}

// Plan runs one planning round. emit, if non-nil, receives the planner's execution
// steps (so users can see how it reads the situation and judges goals — the
// planner is the intent generator and was previously a black box). Returns whether
// the planner judged the goal met.
// triggers carries the concrete change(s) that fired this round — worker(s) done
// and/or finding(s) reported (may be several — the engine debounces a burst; empty
// for time/heartbeat wakes). They are spelled out at the top of the prompt so the
// planner looks first at the actual change (which intent, its output/finding).
func (p *Planner) Plan(ctx context.Context, taskID int64, as *db.AssetStore, ts *db.ExplorationStore, goal string, triggers []TriggerEvent, emit func(db.Activity)) (met bool, reason string, err error) {
	// cold-digest §2.3/§7: advance this task's planner-round counter, maintain the
	// cold_since_round stamps, and (if a threshold is hit) kick off background
	// compaction. Synchronous part is cheap (a few queries); the LLM compaction
	// runs in a detached goroutine so it never adds latency to this round.
	p.compactor.OnPlannerRound(ctx, ts)
	tsx := NewToolSet(ts, "planner")
	tsx.SetFindingRecorder(p.findingRecorder)
	if as != nil {
		tsx.SetAssetStore(as, as.Companies())
	}
	tsx.SetTaskID(taskID)
	tsx.SetCoverageEnabled(as == nil || as.CoverageEnabled(taskID))
	tsx.killWork = p.killWork   // enable kill_work tool (nil = unavailable)
	tsx.steerWork = p.steerWork // enable steer_work tool (nil = unavailable)
	if origin, _ := ts.OriginFactID(); origin > 0 {
		tsx.SetOwnerNode(origin) // planner-side anchors default to the task root (origin fact)
	}
	//Domain tools + basic default toolset (Read/Write/Edit/MultiEdit/LS/Glob/Grep/Bash)
	//Remove add_task_scope/list_untested_assets (without prompt) when asset coverage function is turned off.
	base := append(tsx.DropCoverageTools(tsx.PlannerTools()), actool.DefaultTools()...)
	ctx = WithRunInfo(ctx, RunInfo{TaskID: taskID, ExplorationID: explorationID(ts)})
	tools, def, cleanup := AugmentTools(ctx, "planner", base)
	defer cleanup()
	//The key situation (just completed intention + prefetched complete picture) is changed to [this round of user input] (see input below), system
	//Only the static planning text is left. move-out makes the system stable in each round and more conducive to caching; the price is that if a single round becomes longer, the situation may
	//Compressed by compaction (planner rounds are usually short and low risk). situational will be spelled into the input below.
	situational := renderTriggers(ts, triggers) + renderGraphOverview(tsx.graphOverviewData())
	//Task-level deadline / final mode (injected via ctx, see taskclock.go). In the final round, the task times out
	//The ending word of planner is used as [operation command for this round] and is inserted into the user input for this round (situational), so that it can only be done at the end.
	//Goal determination, no new intentions.
	tc := taskClockFrom(ctx)
	if tc.Final {
		situational += "\n\n[Task timeout wrap-up (special instructions for this round; override the normal planning process above)]: " + resolveTaskTimeoutWrapup("planner")
	}
	//The working directory of this task, <workDir>/tasks/<taskID>, is created first.
	taskDir := ensureRunDir(p.workDir, taskID, 0)
	ctx = intercept.WithReviewContext(ctx, taskDir, intercept.ReviewBackground{})
	sysBody := plannerSystem(goal, p.workDir, taskDir)
	if p.wantConstraints() {
		sysBody += constraintBlock(ts) //Operational constraints (if any) are injected into the system prompts to frame the exploration boundaries.
	}
	system, boundary := deferredSystem(sysBody, def)
	//The planner does not have its own wall clock budget; when there is a deadline, MaxDuration is clamped to the remainder, so that the running planning wheel can
	//The task ends when it reaches the end (because of timeout → task timeout word, because of number of steps → per-run word).
	maxDur, clamped := clampMaxDuration(tc.DeadlineUnix, 0)
	settle := wrapupSettlement("planner", nil)
	if tc.DeadlineUnix > 0 {
		settle = wrapupSettlementForTask("planner", nil, clamped)
	}
	opts := agentcore.Options{
		Provider:        p.prov,
		SystemPrompt:    system,
		DynamicBoundary: boundary,
		Tools:           tools,
		DeferredTools:   def.Deferred,
		UnlockSet:       def.Unlock,
		PermissionMode:  permission.ModeBypass,
		EnableWebFetch:  true, //Record proxy traces; load proxy CA to verify MITM re-signed HTTPS certificate
		WebFetchProxy:   p.proxyAddr,
		WebFetchCACert:  p.proxyCACert,
		//Internet search (optional). ddgs does not require a key; brave-free requires a BraveKey; tavily requires a TavilyKey.
		//WebSearchProxy is an independent export proxy (http/https/socks5) and has nothing to do with the MITM proxy that records traffic; if empty, it is directly connected.
		EnableWebSearch:       p.webSearch.Enabled,
		WebSearchBackend:      p.webSearch.Backend,
		BraveSearchAPIKey:     p.webSearch.BraveKey,
		TavilySearchAPIKey:    p.webSearch.TavilyKey,
		DeepSeekSearchBaseURL: p.webSearch.DeepSeekBaseURL,
		DeepSeekSearchAPIKey:  p.webSearch.DeepSeekAPIKey,
		DeepSeekSearchModel:   p.webSearch.DeepSeekModel,
		WebSearchProxy:        p.webSearch.Proxy,
		BashEnv:               proxyEnv(p.proxyAddr, p.proxyCACert), //Bash subcommand defaults to proxy + trusted CA
		WorkingDir:            taskDir,                              //The working directory of this task <workDir>/tasks/<taskID>
		ToolOutputDir:         cmdOutDir(taskDir),
		MaxTurns:              p.maxTurns, // 0 = unlimited (configurable in agent management)
		MaxDuration:           maxDur,     //0=no limit; when there is a deadline=remaining time before the deadline
		Compaction:            compactionConfig(p.compactionWindow()),
		//Planning backlog shared across wakes: Let the serial chain persist across rounds (session is new, store is not).
		Todos: p.todoFor(ts.ID()),
		//Hit the step budget [this round] → SDK run to the end: implement the conclusions that have been thought out clearly in this round (the faction’s add_intent,
		//Provable prove_goal, serial chain record TodoWrite), rather than stopping planning - the planner will still be awakened repeatedly afterwards.
		//Use PromptByReason instead (see wrapupSettlementForTask) when clamped (clamped by task deadline).
		Settlement:   settle,
		NonStreaming: p.nonStreaming(), //Provider.Complete is used when the profile is non-streaming.
		MaxTokens:    p.maxTokens(),    //0 = Do not send the upper limit, determined by the server default value
	}
	if p.tx != nil { // persist raw LLM conversation; one accumulating file per task's planner
		opts.Transcript = p.tx
		opts.SessionID = fmt.Sprintf("exp%d-planner", ts.ID())
	}
	//Experimental function: After turning on, noa will take over the context compression (the archives are concentrated under <workDir>/noa/<SessionID> and are persistent).
	noaSession := fmt.Sprintf("exp%d-planner", ts.ID())
	enableNoa(&opts, p.noaEnabledFn, p.workDir, noaSession, noaWarn(noaSession))
	//The situation (just completed intention + complete picture) is now combined into this round of user input (see input below). There are also
	//Command + cross-wakeup to-do (todo is the model’s own planning note, which is reproducible and can be placed in user).
	//The opening remarks are divided into two types according to "whether there are specific changes in this round": there are changes → the [Actual Changes] block pointing below; there are no changes
	//(Heartbeat scheduled inspection / hint / recovery, etc.) → Don't lie and say "the picture has changed" and instead prompt to review the running intention.
	lead := "A concrete change just occurred (see [Changes that triggered this planning round] below). Plan the next step accordingly:"
	if len(triggers) == 0 {
		lead = "This wake-up is a **scheduled check-in (heartbeat) with no concrete change signal**; the graph may not have changed. Also review running intents: use steer_work to correct work that is stalled or drifting, and kill_work if its direction is fundamentally wrong. Then assess goals and decide whether to add directions:"
		//When waking up with no change in heartbeat, if there is no open or running intention in the entire map → exploration has stopped (no workers are running,
		//There is no queue direction either). Clearly inform the planner and force it to make up a new direction this round. Don't just review the running intention and then idle for one round.
		if active, err := ts.HasActiveIntent(); err == nil && !active {
			lead = "This wake-up is a **scheduled check-in (heartbeat)** and there are **no open or running intents**. No Worker is running and no direction is queued, so exploration has stalled. You **must** produce at least one new intent that advances a goal and **does not duplicate** an intent already in the graph; do not produce zero intents. First use the situation below to assess whether goals are met, then add directions immediately if any remain unmet:"
		}
	}
	input := lead + situational + "\n\nAssess goals based on the situation above. When a goal is **actually met** (the outcome has been obtained or a vulnerability relevant to the goal has been confirmed), mark it individually with prove_goal. **Hard requirement: if any goal remains unmet and there are no open or running intents (frontier_open=0 and running_intents is empty), produce at least one intent that advances a goal this round. With no running work to await and no queued direction, zero intents stalls the task. You may produce no new intents only when open/running intents are making progress or all goals are met.**" +
		renderPlannerTodos(opts.Todos.List())
	//MaxDuration will now interrupt the running tool when the wall clock reaches the point and finish it on the spot (on the live ctx), no longer stuck in a single round
	//Bypass the finish and eliminate the need for external hard CTX pockets. ctx only carries pause/kill/shutdown.
	_, _, err = captureRun(ctx, opts, input,
		func(r db.Activity) {
			if emit != nil {
				r.Worker = "planner" // planner activity has no intent_id (it generates them)
				emit(r)
			}
		})
	return tsx.GoalMet, tsx.Reason, err
}
