package agent

import (
	"context"
	"fmt"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
)

// MainAgent is the thin human-interface orchestrator (docs §4.2 / §7). The human
// chats with it; it observes (read tools), and steers by injecting hints
// (→planner) or direct high-priority intents (→frontier). It does NOT run the
// autonomous intent-generation loop (that is the planner's job).
type MainAgent struct {
	findingRecorder FindingRecorder
	prov            llm.Provider
	model           string
	tx              *transcript.Store                      // raw LLM conversation persistence (nil = off)
	window          int                                    // context window in tokens (for compaction)
	windowFn        func() int                             // optional dynamic task-chain minimum
	maxTurns        int                                    // max agent turns per run (0 = unlimited)
	proxyAddr       string                                 // recording proxy for WebFetch (empty = direct)
	proxyCACert     string                                 // recording proxy's CA cert path (HTTPS verify)
	webSearch       WebSearchOpts                          // web_search tool backend selection (off by default)
	workDir         string                                 // shared work dir (surfaced in prompt as artifact-output target)
	steerWork       func(intentID int64, msg string) error // engine callback: steer a running work (nil = off)
	nonStreamingFn  func() bool                            // resolver: use non-streaming (Complete) path? (nil = streaming)
	noaEnabledFn    func() bool                            // resolver: use experimental noa compaction? (nil = off)
	maxTokensFn     func() int                             // resolver: per-reply output cap (nil/0 = send no cap)
}

// SetNoaEnabled wires a resolver deciding whether runs use the experimental noa
// context-compression mechanism. nil/unset = off (built-in compaction). Read per
// run so the settings toggle takes effect without rebuilding the agent.
func (m *MainAgent) SetNoaEnabled(fn func() bool) { m.noaEnabledFn = fn }

// SetNonStreaming wires a resolver deciding whether runs use the non-streaming
// model path (true = non-streaming). nil/unset = streaming (default).
func (m *MainAgent) SetNonStreaming(fn func() bool) { m.nonStreamingFn = fn }

func (m *MainAgent) nonStreaming() bool { return m.nonStreamingFn != nil && m.nonStreamingFn() }

// SetMaxTokens wires a resolver for the per-reply output cap. nil/unset or 0 =
// send no cap and let the endpoint decide. Read per run, like nonStreaming.
func (m *MainAgent) SetMaxTokens(fn func() int) { m.maxTokensFn = fn }

func (m *MainAgent) maxTokens() int {
	if m.maxTokensFn == nil {
		return 0
	}
	return m.maxTokensFn()
}

func NewMainAgent(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int) *MainAgent {
	return &MainAgent{prov: prov, model: model, workDir: workDir, tx: tx, window: window, maxTurns: maxTurns}
}

func (m *MainAgent) SetCompactionWindowResolver(fn func() int) { m.windowFn = fn }

func (m *MainAgent) compactionWindow() int {
	if m.windowFn != nil {
		return m.windowFn()
	}
	return m.window
}

// SetProxy points the main agent's WebFetch at the recording proxy plus the CA
// cert it trusts to verify HTTPS through it (empty addr = direct).
func (m *MainAgent) SetProxy(addr, caCert string) { m.proxyAddr, m.proxyCACert = addr, caCert }

// SetWebSearch selects the web_search backend for the main agent (off by default).
func (m *MainAgent) SetWebSearch(o WebSearchOpts) { m.webSearch = o }

// SetSteerWork wires the engine callback that lets the main agent's steer_work
// tool inject a mid-run course-correction into a running work (nil = tool off).
func (m *MainAgent) SetSteerWork(fn func(intentID int64, msg string) error) { m.steerWork = fn }

//mainAgentDefaultTmpl is the built-in EDITABLE body (section [A]) of the main agent
//prompt, seeded into agent_prompts. Goal is a {{.Goal}} template var; the middle
//Artifact output specification tail is code-owned (artifactSpec), appended after rendering.
const mainAgentDefaultTmpl = `You are the "main agent" in an authorized penetration-testing system and the interface for the human operator. You do not conduct exploration yourself or continuously generate intents; that is the planner's job. Your responsibilities:

1. Observe: answer questions about current progress using graph_overview, list_findings, list_facts, list_assets, and get_worker_output.
2. Steer the system according to the user's intent:
   - To change direction, emphasize a vulnerability class, or focus on an area, use add_hint. The planner will read it on its next run.
   - To test a specific target immediately, use add_intent to inject a high-priority intent (priority 8–10). The system automatically resumes a completed task, assigns the intent to a Worker, and returns the task to completed when the intent finishes.
     **When all task goals are already met** (all goals in graph_overview have state=met), first determine whether the requested intent implies a new outcome to achieve. If so, restate the likely goal in one sentence and **ask whether it should be registered as an official goal**. If yes, register it with set_goals; the task enters normal planning and the planner continues autonomously. If not, or if the user only wants a one-time check, dispatch only this intent with add_intent; after the Worker finishes, the task returns to completed and does not continue on its own. If the intent is clearly a one-time verification with no implied new goal, call add_intent directly without asking.
   - To steer a running intent (for example, stop pursuing X and focus on Y), use steer_work. This does not interrupt the Worker or discard its progress; the guidance takes effect before its next action. First use get_worker_output to see what it is doing. If the direction is fundamentally wrong, use add_intent to issue a new one instead.
   - To add a new final outcome, use set_goals. The system adds it to the task graph and **automatically resumes completed or paused tasks** so the planner can reassess progress; the user does not need to resume the task manually.
   - To add or edit testing constraints (allow or prohibit operations, such as "test only the current port," "no brute-force attacks," or "passive reconnaissance only"), use set_constraints (type=allow or type=deny). Constraints are added to planner/Worker prompts on the next planning round to define exploration boundaries. They can also be edited in the Overview's Constraints section.
3. Reply concisely in plain language and explain what you did.

Current task goal: {{.Goal}}

Do not invent findings. Base answers only on data returned by tools.`

func mainAgentSystem(goal, dataDir, workDir string) string {
	body := renderSystem("mainagent", mainAgentDefaultTmpl, MainVars{Goal: goal, DataDir: dataDir, Now: nowStr()})
	return body + artifactSpec(workDir)
}

// Chat handles one human message and returns the assistant reply. emit, if
// non-nil, receives each execution step (thinking / tool_use / tool_result /
// text / result) so the main-agent session shows its work — exactly like the
// worker/planner sessions — not just the final answer.
func (m *MainAgent) Chat(ctx context.Context, taskID int64, mainSeg int, as *db.AssetStore, ts *db.ExplorationStore, goal, message string, emit func(db.Activity), notify, resume func(), notifyGoal, notifyHint func([]string)) (string, error) {
	tsx := NewToolSet(ts, "human")
	tsx.SetFindingRecorder(m.findingRecorder)
	if as != nil {
		tsx.SetAssetStore(as, as.Companies())
	}
	tsx.SetTaskID(taskID)
	tsx.SetCoverageEnabled(as == nil || as.CoverageEnabled(taskID))
	tsx.SetNotify(notify)         //Generic wakeup (write operation without dedicated callback takes it, debounced)
	tsx.SetResumeTask(resume)     //set_goals adds new goals → brings completed/suspended tasks back to running
	tsx.SetNotifyGoal(notifyGoal) //set_goals Add a new goal → Write a note to the planner "People added a new goal:..." trigger
	tsx.SetNotifyHint(notifyHint) //add_hint Add a new hint → Record a "People have added N strategic hints:..." trigger to the planner
	tsx.steerWork = m.steerWork   // enable steer_work tool (nil = unavailable)
	//Domain tools + basic default toolset (Read/Write/Edit/MultiEdit/LS/Glob/Grep/Bash)
	//Remove add_task_scope/list_untested_assets (without prompt) when asset coverage function is turned off.
	base := append(tsx.DropCoverageTools(tsx.MainAgentTools()), actool.DefaultTools()...)
	ctx = WithRunInfo(ctx, RunInfo{TaskID: taskID, ExplorationID: explorationID(ts)})
	tools, def, cleanup := AugmentTools(ctx, "mainagent", base)
	defer cleanup()
	//The working directory of this task, <workDir>/tasks/<taskID>, is created first.
	mainDir := ensureRunDir(m.workDir, taskID, 0)
	ctx = intercept.WithReviewWorkingDirectory(ctx, mainDir)
	system, boundary := deferredSystem(mainAgentSystem(goal, m.workDir, mainDir), def)
	opts := agentcore.Options{
		Provider:        m.prov,
		SystemPrompt:    system,
		DynamicBoundary: boundary,
		Tools:           tools,
		DeferredTools:   def.Deferred,
		UnlockSet:       def.Unlock,
		PermissionMode:  permission.ModeBypass,
		EnableWebFetch:  true, //Record proxy traces; load proxy CA to verify MITM re-signed HTTPS certificate
		WebFetchProxy:   m.proxyAddr,
		WebFetchCACert:  m.proxyCACert,
		//Internet search (optional). ddgs does not require a key; brave-free requires a BraveKey; tavily requires a TavilyKey.
		//WebSearchProxy is an independent export proxy (http/https/socks5) and has nothing to do with the MITM proxy that records traffic; if empty, it is directly connected.
		EnableWebSearch:       m.webSearch.Enabled,
		WebSearchBackend:      m.webSearch.Backend,
		BraveSearchAPIKey:     m.webSearch.BraveKey,
		TavilySearchAPIKey:    m.webSearch.TavilyKey,
		DeepSeekSearchBaseURL: m.webSearch.DeepSeekBaseURL,
		DeepSeekSearchAPIKey:  m.webSearch.DeepSeekAPIKey,
		DeepSeekSearchModel:   m.webSearch.DeepSeekModel,
		WebSearchProxy:        m.webSearch.Proxy,
		BashEnv:               proxyEnv(m.proxyAddr, m.proxyCACert), //Bash subcommand defaults to proxy + trusted CA
		WorkingDir:            mainDir,                              //The working directory of this task <workDir>/tasks/<taskID>
		ToolOutputDir:         cmdOutDir(mainDir),
		MaxTurns:              m.maxTurns,                             // 0 = unlimited (configurable in agent management)
		Compaction:            compactionConfig(m.compactionWindow()), // long chats stay within the window
		Todos:                 actool.NewTodoStore(),                  //Session-level temporary to-do (TodoWrite), purely for planning purposes, will be lost upon exit.
		//Hit budget (number of steps) → SDK run ending: output a progress summary to the user. Prompt and the number of closing rounds can be edited in the background (default is 10 rounds).
		Settlement:   wrapupSettlement("mainagent", nil),
		NonStreaming: m.nonStreaming(), //Provider.Complete is used when the profile is non-streaming.
		MaxTokens:    m.maxTokens(),    //0 = Do not send the upper limit, determined by the server default value
	}
	if m.tx != nil { // persist raw human↔AI conversation; one accumulating file per segment
		opts.Transcript = m.tx
		// Segment 0 keeps the legacy "exp%d-main" name so existing transcripts still
		// load; each new session (seg>=1) gets its own file for a clean context.
		opts.SessionID = fmt.Sprintf("exp%d-main", ts.ID())
		if mainSeg > 0 {
			opts.SessionID = fmt.Sprintf("exp%d-main-s%d", ts.ID(), mainSeg)
		}
	}
	//Experimental function: After turning on, noa will take over the context compression (the archives are concentrated under <workDir>/noa/<SessionID> and are persistent).
	//Session id and transcript have the same rules (segment awareness), so that archiving and restoration are aligned.
	noaSession := fmt.Sprintf("exp%d-main", ts.ID())
	if mainSeg > 0 {
		noaSession = fmt.Sprintf("exp%d-main-s%d", ts.ID(), mainSeg)
	}
	enableNoa(&opts, m.noaEnabledFn, m.workDir, noaSession, noaWarn(noaSession))
	ctx = attachSideCapture(ctx, &opts)
	s := agentcore.NewSession(opts)
	defer s.Close()
	// reload the prior conversation from the transcript so the agent has context
	// across turns (each Chat is a fresh session; without this it can't see earlier
	// messages). First turn: no file yet → Resume loads nothing and proceeds.
	if m.tx != nil {
		_ = s.Resume(opts.SessionID)
	}
	// C2: this session is fresh each turn; re-unlock skill-gated MCPs from prior
	// Skill() calls in the reloaded history so revealed tools stay callable.
	seedUnlockFromHistory(s.Messages(), def.UnlockSkill)
	text, _, err := captureRunSession(ctx, s, message, func(r db.Activity) {
		if emit != nil {
			r.Worker = "mainagent"
			emit(r)
		}
	})
	return text, err
}
