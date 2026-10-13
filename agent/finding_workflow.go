package agent

import (
	"encoding/json"
	"fmt"

	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

const findingIDGuidance = "\n\n**Finding ID conventions**: finding_id is the standalone finding-record ID; finding_node_id is the exploration-node ID. The id field in list_findings / list_task_findings / node_detail / get_task_node_detail remains an exploration-node ID; read the standalone ID from the finding_id field in the same result. get_finding_traffic / bind_finding_traffic use the standalone finding_id. The legacy finding_id argument of update_finding_report still expects finding_node_id. Do not use the number in report_finding's first line for evidence tools, and do not guess other IDs after an ID error."

// The server supplies the persisted setting. A missing setting/host is off.
// Consulted at assembly and again on writes so an already-running session
// cannot keep binding after the user switches the feature off.
var FindingTrafficBindingEnabled func() bool

func findingTrafficBindingEnabled() bool {
	return FindingTrafficBindingEnabled != nil && FindingTrafficBindingEnabled()
}

// Applied after ToolResolve: user descriptions and prompts remain intact, while
// all actual reporters (including Planner and custom chat agents) see the same
// API contract. Disabled/unbound tools are never reintroduced here.
func findingWorkflowTools(agentKey string, tools []actool.CoreTool) ([]actool.CoreTool, string) {
	if !findingTrafficBindingEnabled() {
		out := make([]actool.CoreTool, 0, len(tools))
		for _, tool := range tools {
			if tool.Name() == "bind_finding_traffic" {
				continue
			}
			if agentKey == "reporter" && (tool.Name() == "traffic_search" || tool.Name() == "traffic_get" || tool.Name() == "traffic_blob") {
				continue
			}
			switch tool.Name() {
			case "report_finding", "add_hint", "add_task_hint":
				// Work on a copy: toggling back on must restore the original schema.
				raw, _ := json.Marshal(tool.InputSchema())
				var schema map[string]any
				if json.Unmarshal(raw, &schema) == nil {
					stripTrafficParameters(schema)
					tool = DecorateTool(tool, tool.Description(), schema)
				}
			}
			out = append(out, tool)
		}
		return out, ""
	}
	out := append([]actool.CoreTool(nil), tools...)
	has := map[string]bool{}
	for i, tool := range out {
		has[tool.Name()] = true
		note := ""
		switch tool.Name() {
		case "report_finding":
			note = "\nBy default, the report writer verifies and binds traffic before writing the report. Keep verification commands, key output, existing real traffic IDs, and their purposes in evidence so the report writer can cross-check execution records; extra traffic lookups solely for binding are unnecessary. Explicit immediate binding is still supported: traffic_refs or evidence_hint_id may provide verified references, with the latter reading structured references from the specified local hint. Any invalid reference causes the entire submission to fail. These optional parameters are unnecessary for TCP findings or missing traffic. Returned finding_id and finding_node_id identify the standalone record and exploration node respectively."
		case "add_hint", "add_task_hint":
			note = "\nWhen handing off a confirmed finding, preserve verified traffic IDs, roles, notes, and order in the relevant hint's traffic_refs (top-level for one hint, or the corresponding hints element for a batch). Explain in text which specific finding the traffic proves. Do not discard existing traffic references and pass only text. Unverified candidates cannot be passed as evidence."
		case "get_finding_traffic", "bind_finding_traffic", "list_findings", "list_task_findings", "node_detail", "get_task_node_detail", "update_finding_report":
			note = findingIDGuidance
		}
		if note != "" {
			out[i] = DecorateTool(tool, tool.Description()+note, tool.InputSchema())
		}
	}
	guidance := ""
	if has["report_finding"] || has["add_task_hint"] || has["add_hint"] {
		guidance = "\n\n**Traffic evidence handoff (optional)**: by default, the report writer binds traffic after the finding is registered and before writing its report. Preserve verification commands, key output, existing real traffic IDs, and their purposes in evidence, and include intent_id in task context for tracing. Extra lookups solely for binding are unnecessary. Auto / Planner must preserve the executor's existing references when reporting on its behalf. add_hint / add_task_hint accept traffic_refs for handoff; report_finding still supports explicit immediate binding via traffic_refs / evidence_hint_id. Register TCP findings or findings without traffic normally; do not guess IDs or probe again just to collect missing traffic."
		if has["add_task_hint"] && !has["add_hint"] {
			guidance += "\nIn platform chat without task context, do not call report_finding directly. Hand off to an existing relevant task with add_task_hint so its agent can register the finding, then check the result with list_task_findings."
		}
		if has["prove_goal"] || has["goal_met"] {
			guidance += "\nReport or hand off all existing evidence from this run before assessing goal completion. Do not finish the task or cancel workers merely because a text finding has been registered while evidence handoff remains incomplete. Missing traffic does not require waiting or forcing capture."
		}
	}
	if has["update_finding_report"] && has["bind_finding_traffic"] && has["get_finding_traffic"] {
		guidance += "\n\n**Automatic traffic binding before reporting (enabled)**: verify and bind traffic for the finding that triggered this run before writing its report. Obtain explicit finding_id and finding_node_id values from report_finding JSON or get_task_node_detail / list_task_findings. Read the finding details, related intent trace, and current evidence list; prefer real IDs handed off by the reporter. If verification used HTTP and traffic tools are available, filter candidates with traffic_search, then use traffic_get to verify that each request/response actually supports this finding. Domain and time filter candidates but do not prove ownership. Bind confirmed evidence in reproduction order with bind_finding_traffic(finding_id, traffic_refs), choosing baseline / proof / verification / supporting and explaining its purpose. Work only on this finding; do not register it again or probe the target again. After binding succeeds, call get_finding_traffic again for the latest version, read the needed bodies, and pass the version actually read as evidence_version to update_finding_report (whose finding_id argument still expects finding_node_id). Do not add existing bindings again. For TCP findings, missing capture, unavailable tools, or no exact match, skip automatic binding and write the report from text/command evidence, explaining the reason. Never guess traffic IDs to complete a list. Do not claim binding succeeded if it failed; retain existing evidence and explain missing bindings in the report."
	}
	if guidance != "" || has["get_finding_traffic"] || has["update_finding_report"] {
		guidance += findingIDGuidance
	}
	return out, guidance
}

func stripTrafficParameters(schema map[string]any) {
	props, _ := schema["properties"].(map[string]any)
	delete(props, "traffic_refs")
	delete(props, "evidence_hint_id")
	if required, ok := schema["required"].([]any); ok {
		kept := required[:0]
		for _, key := range required {
			if key != "traffic_refs" && key != "evidence_hint_id" {
				kept = append(kept, key)
			}
		}
		schema["required"] = kept
	}
	if hints, ok := props["hints"].(map[string]any); ok {
		if items, ok := hints["items"].(map[string]any); ok {
			stripTrafficParameters(items)
		}
	}
}

// HintTrafficSchema is shared by the task-local and cross-task hint tools.
func HintTrafficSchema() map[string]any {
	return map[string]any{"type": "array", "description": "Optional verified traffic references supporting the specific finding in this hint, in order. After handoff, report_finding can use evidence_hint_id to include these references.", "items": obj(map[string]any{"traffic_id": str("Real traffic ID"), "role": str("baseline / proof / verification / supporting"), "note": str("Conclusion supported by this traffic")}, "traffic_id")}
}

func (t *ToolSet) findingRefsFromHint(hintID int64, explicit []db.TrafficRef) ([]db.TrafficRef, error) {
	if hintID <= 0 {
		return db.NormalizeTrafficRefs(explicit)
	}
	n, err := t.ts.GetNode(hintID) // local store only: inherited hints cannot supply evidence
	if err != nil {
		return nil, err
	}
	if n == nil || n.Kind != db.KindHint {
		return nil, fmt.Errorf("evidence_hint_id=%d must identify a hint node in this task (inherited hints cannot be used directly for binding)", hintID)
	}
	var payload struct {
		Refs []db.TrafficRef `json:"traffic_refs"`
	}
	if err := json.Unmarshal(n.Payload, &payload); err != nil {
		return nil, err
	}
	return db.NormalizeTrafficRefs(append(append([]db.TrafficRef{}, explicit...), payload.Refs...))
}
