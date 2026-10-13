package agent

import (
	"context"
	"errors"
	"fmt"
)

// AbortCause names why an agent run's context was cancelled. Every cancellation
// site should attach one so the activity trace can report the real initiator.
type AbortCause struct {
	Code  string
	Short string
	Text  string
}

func (c *AbortCause) Error() string { return c.Text }

func cause(code, short, text string) *AbortCause {
	return &AbortCause{Code: code, Short: short, Text: text}
}

// Causef builds a cause that includes runtime-specific detail.
func Causef(code, short, format string, args ...any) *AbortCause {
	return &AbortCause{Code: code, Short: short, Text: fmt.Sprintf(format, args...)}
}

var (
	// Task-level execution context.
	AbortPausedByUser = cause("paused_by_user", "Task paused by user",
		"The user paused the task through the task control endpoint (POST /api/tasks/{id}/control, action=pause). The Planner/Worker run was cancelled; running intents return to the open frontier and will be claimed and restarted when the task resumes.")
	AbortPausedByOrchestrator = cause("paused_by_orchestrator", "Task paused by orchestration agent",
		"The orchestration agent paused this task using the pause_task tool. The Planner/Worker run was cancelled; running intents return to the open frontier and will restart when the task resumes.")
	AbortTaskDeleted = cause("task_deleted", "Task deleted",
		"The task is being deleted (DELETE /api/tasks/{id}). The deletion barrier cancelled its running Planner, Workers, and main agent; results from this run will not be used.")
	AbortPausedOnReload = cause("paused_on_reload", "Task pause state restored on startup",
		"On startup, the backend restored the task's paused state from the database. This run was cancelled; normally, no agents are running during restoration.")
	AbortGoalMet = cause("goal_met", "Planner: task goals met",
		"The planner determined that the task goals were met and marked the task done, then cancelled any remaining Workers. Their intents are marked stopped, not failed.")
	AbortSettleDrainTimeout = cause("settle_drain_timeout", "Task timeout wrap-up wait expired",
		"After the task timed out, the system waited for running Workers to wrap up gracefully. The 90-second drain period was insufficient, so they were force-cancelled. Intents are marked exhausted; facts and assets written during wrap-up are preserved.")

	// Per-work context.
	AbortKilledByPlanner = cause("killed_by_planner", "Intent stopped by planner",
		"The planner stopped this intent using kill_work, usually because the direction was off track or no longer useful. The intent is marked stopped and will not be claimed again automatically.")
	AbortWorkPausedByUser = cause("work_paused_by_user", "Worker intent paused by user",
		"The user paused the running Worker. This call was cancelled and the intent is now paused. Registered intents, facts, findings, and activity are preserved; execution restarts from the beginning when resumed.")
	AbortWorkCancelledByUser = cause("work_cancelled_by_user", "Worker intent deleted by user",
		"The user deleted the running Worker intent, so this call was cancelled. After the Worker exits its write section, the server applies the selected deletion mode: soft deletion marks it deleted and preserves its output; hard deletion cascades to the intent and downstream nodes supported only by it.")
	AbortWorkFinished = cause("work_finished", "Worker finished and released its context",
		"The Worker finished normally and the engine released its context in detachWork. This is not an interrupted run; if it appears in an interruption message, cancellation and completion raced.")
	AbortPausedRaceGuard = cause("paused_race_guard", "New run rejected while task is paused",
		"The engine refused to create an execution context while the task is paused. This prevents a race between claiming work and pausing the task from starting another Worker. Claimed intents return to the frontier.")

	// Main Agent and standalone conversation contexts.
	AbortChatStoppedByUser = cause("chat_stopped_by_user", "Conversation stopped by user",
		"The user clicked Stop and cancelled this main-agent or conversation-agent run. Existing activity is preserved, and another message can be sent.")
	AbortChatPausedWithTask = cause("chat_paused_with_task", "Task paused; main chat cancelled",
		"The running main-agent conversation was also cancelled when the user paused the task. Existing activity is preserved; this turn will not be replayed automatically when the task resumes.")
	AbortChatTurnFinished = cause("chat_turn_finished", "Chat turn finished; context released",
		"This conversation turn finished normally, and the server is releasing its context. This is not an interrupted run; if it appears in an interruption message, cancellation and completion raced.")

	// Process-level and per-run hard backstop.
	AbortShutdown = cause("shutdown", "Backend process shutting down",
		"The backend received SIGINT or SIGTERM and is restarting, updating, or shutting down. All running agents will be cancelled; after restart, any remaining running intents will reset to open and run again.")
	AbortRunHardTimeout = cause("run_hard_timeout", "Per-run hard timeout triggered",
		"The run exceeded its soft wall-clock budget and extra grace period. A model request or tool may have stopped responding, preventing normal turn-boundary wrap-up. Check the last tool call that had not returned before the interruption.")
)

// AbortReason resolves the named cause attached to a cancelled run context.
func AbortReason(ctx context.Context) (code, short, text string, ok bool) {
	c := context.Cause(ctx)
	if c == nil {
		return "", "", "", false
	}
	var ac *AbortCause
	if errors.As(c, &ac) {
		return ac.Code, ac.Short, ac.Text, true
	}
	switch {
	case errors.Is(c, context.DeadlineExceeded):
		return "deadline_exceeded", "Upstream context deadline exceeded",
			"The upstream context deadline was reached, but the caller did not attach a named cause with WithTimeoutCause: " + c.Error(), true
	case errors.Is(c, context.Canceled):
		return "canceled_no_cause", "Cancelling caller did not attach a named cause",
			"The upstream context was cancelled without a named cause from context.WithCancelCause. Register the cause in agent/cancelcause.go and attach it at the cancellation point.", true
	default:
		return "other", firstLine(c.Error(), 80), c.Error(), true
	}
}
