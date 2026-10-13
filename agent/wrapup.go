package agent

import (
	"strings"

	"github.com/Autumn-27/norma/harness"
)

//Closing prompt (wrap-up / settlement prompt): When the agent runs out of steps (MaxTurns) or
//When [Timeout (run_seconds/MaxDuration)] is terminated, the settlement phase of the SDK will inject this prompt.
//Let the agent first store the content that has been identified but not written back, and then output a summary to avoid unfinished work.
//
//The closing prompt of each agent can be overwritten on demand in the background (save agents.wrapup_prompt). If left blank, use here
//Built-in default. Only [prompt text] can be edited; which tools to disable and how many rounds of budgets to end with are code fixed strategies.

// WrapupOverride, if set, returns the stored wrap-up prompt for an agent key and
// whether a non-empty one exists. Wired by the server to the agents table (like
// PromptOverride for system prompts). nil / empty → the built-in default is used.
var WrapupOverride func(agentKey string) (string, bool)

// WrapupMaxTurnsOverride, if set, returns the admin-configured turn budget for the
// wrap-up phase of an agent and whether a positive one exists. Wired to the agents
// table. nil / ≤0 → the built-in per-agent default (wrapupTurnDefaults) is used.
var WrapupMaxTurnsOverride func(agentKey string) (int, bool)

//Built-in default closing prompts, indexed by agent key. Workers reuse the historically hard-coded settleWrapUpPrompt
//(Defined in worker.go), planner/mainagent each has its own version; the missing one (custom agent) is the universal one.
var wrapupDefaults = map[string]string{
	"worker":    settleWrapUpPrompt,
	"planner":   plannerWrapUpDefault,
	"mainagent": mainAgentWrapUpDefault,
}

//wrapupTurnDefaults: The built-in default round budget of each agent's closing stage [self] (can be overridden by background >0).
//Each game is given 10 rounds to ensure that there are enough moves in the final stage. Missed away genericWrapupTurns.
var wrapupTurnDefaults = map[string]int{
	"worker":    10,
	"planner":   10,
	"mainagent": 10,
}

const genericWrapupTurns = 10

const plannerWrapUpDefault = "This planning round is about to reach its step limit. Only this round is ending; the system will wake you again as the situation changes. You do not need to end the overall planning process. Save the conclusions you have reached, without inventing intents just to fill a wrap-up (zero intents is a valid result). (1) If you identified exploration directions that should be assigned now, submit them together with add_intent. (2) For goals already proven by findings/facts, call prove_goal to mark them met. (3) If a serial exploitation chain needs multiple steps, record it with TodoWrite so later rounds can continue assigning them. Then end this round without a summary."

const mainAgentWrapUpDefault = "Your step budget is nearly exhausted and this interaction is ending. Do not start new exploration or operations. Finish with one standalone plain-text sentence summarizing current progress, key conclusions, and the recommended next step."

const genericWrapUpDefault = "You are about to be stopped because your budget is exhausted. First save completed results that have not yet been recorded, then finish with one standalone plain-text sentence summarizing what you did and the key conclusions. This sentence will be displayed as the run result."

// WrapupDefault returns the built-in default wrap-up prompt for an agent key —
// used by the admin UI as the "restore default" value and empty-field placeholder.
func WrapupDefault(agentKey string) string {
	if d, ok := wrapupDefaults[agentKey]; ok {
		return d
	}
	return genericWrapUpDefault
}

// WrapupTurnsDefault returns the built-in wrap-up turn budget for an agent key —
// used by the admin UI as the "0 = default N" hint.
func WrapupTurnsDefault(agentKey string) int {
	if n, ok := wrapupTurnDefaults[agentKey]; ok {
		return n
	}
	return genericWrapupTurns
}

// resolveWrapup returns the effective wrap-up prompt: the DB override (if set and
// non-empty) over the built-in default.
func resolveWrapup(agentKey string) string {
	if WrapupOverride != nil {
		if t, ok := WrapupOverride(agentKey); ok && strings.TrimSpace(t) != "" {
			return t
		}
	}
	return WrapupDefault(agentKey)
}

// resolveWrapupTurns returns the effective wrap-up turn budget: a positive DB
// override over the built-in per-agent default.
func resolveWrapupTurns(agentKey string) int {
	if WrapupMaxTurnsOverride != nil {
		if v, ok := WrapupMaxTurnsOverride(agentKey); ok && v > 0 {
			return v
		}
	}
	return WrapupTurnsDefault(agentKey)
}

// wrapupSettlement builds the settlement config for an agent's run. Prompt and the
// turn budget are admin-editable per agent; disabled tools are code-owned policy so
// a user can't edit away the "stop probing" guardrail. Resolved fresh each run
// (reads DB live), so edits apply on the next run without a restart.
func wrapupSettlement(agentKey string, disabledTools []string) *harness.Settlement {
	return &harness.Settlement{
		Prompt:        resolveWrapup(agentKey),
		DisabledTools: disabledTools,
		MaxTurns:      resolveWrapupTurns(agentKey),
	}
}

//---------- Task-level timeout closing words (see docs/task-level timeout and closing design.md) ----------
//
//The ending words with per-run are [two sets]: per-run is "your budget for this run has been used up"; task timeout is
//"The entire mission is coming to an end." The semantics are often opposite (especially planner: per-run says "don't stop planning",
//Task timeout says "stop planning and make final judgment when the point is reached"). Only configured for worker/planner.

//WrapupTaskTimeoutOverride / ...TurnsOverride: DB override of task timeout ending word and round number
//(wire to agents.task_timeout_wrapup_prompt/_max_turns, worker/planner only).
var (
	WrapupTaskTimeoutOverride      func(agentKey string) (string, bool)
	WrapupTaskTimeoutTurnsOverride func(agentKey string) (int, bool)
)

var taskTimeoutWrapupDefaults = map[string]string{
	"worker":  workerTaskTimeoutDefault,
	"planner": plannerTaskTimeoutDefault,
}

const workerTaskTimeoutDefault = "**The entire task has reached its timeout and is ending.** This is the deadline for the whole exploration, rather than just this run. This is your final opportunity: (1) Save all results you have identified but not yet recorded: insert_assets for new assets, record_fact for exploration conclusions/facts, and report_finding for confirmed findings. (2) Do not start new commands or probes. (3) Finish with one standalone plain-text sentence summarizing the key conclusions for this intent."

const plannerTaskTimeoutDefault = "**The entire task has reached its timeout and is ending.** This is the end of the task, rather than only this round. Make a final goal assessment using all current facts and findings. Call prove_goal to mark every goal supported by evidence as met. Do not generate new intents; they would not be executed now. End after the assessment without a summary."

//TaskTimeoutWrapupDefault returns the built-in default ending word of an agent's task timeout (for background placeholder/restore default).
func TaskTimeoutWrapupDefault(agentKey string) string {
	return taskTimeoutWrapupDefaults[agentKey] //Not configured (mainagent/chat) returns an empty string
}

//resolveTaskTimeoutWrapup: DB override (non-null) > built-in default. An empty string indicates that the agent has no task timeout word.
//(non-worker/planner), the caller should fallback per-run words at this time.
func resolveTaskTimeoutWrapup(agentKey string) string {
	if WrapupTaskTimeoutOverride != nil {
		if t, ok := WrapupTaskTimeoutOverride(agentKey); ok && strings.TrimSpace(t) != "" {
			return t
		}
	}
	return TaskTimeoutWrapupDefault(agentKey)
}

func resolveTaskTimeoutTurns(agentKey string) int {
	if WrapupTaskTimeoutTurnsOverride != nil {
		if v, ok := WrapupTaskTimeoutTurnsOverride(agentKey); ok && v > 0 {
			return v
		}
	}
	return resolveWrapupTurns(agentKey) //The per-run number of rounds is used by default
}

// wrapupSettlementForTask builds settlement for a worker/planner run that is aware
// of the task deadline. See §5 of the design doc:
//- clamped=true → This run is clamped by the task deadline: due to Timeout ending = task deadline → task timeout word;
//Because MaxTurns ends = the number of steps in the pinch window is exhausted first, and there are still a few minutes left in the task → fall back to the per-run word.
//- clamped=false → It's still early in the task: both reasons use per-run words (i.e. degenerate into wrapupSettlement).
//
//The PromptByReason given to the harness is selected on-site according to the [actual] reason at the end, and is mismatched when there is no build.
func wrapupSettlementForTask(agentKey string, disabledTools []string, clamped bool) *harness.Settlement {
	perRun := resolveWrapup(agentKey)
	st := &harness.Settlement{
		Prompt:        perRun, //Back to the bottom (also the values ​​of the two reasons when not clamped)
		DisabledTools: disabledTools,
		MaxTurns:      resolveWrapupTurns(agentKey),
	}
	if clamped {
		if tt := resolveTaskTimeoutWrapup(agentKey); tt != "" {
			st.PromptByReason = map[harness.TerminalReason]string{
				harness.ReasonTimeout:  tt,     //Mission accomplished
				harness.ReasonMaxTurns: perRun, //The number of steps is exhausted first, and there is still time left in the task.
			}
			st.MaxTurns = resolveTaskTimeoutTurns(agentKey)
		}
	}
	return st
}
