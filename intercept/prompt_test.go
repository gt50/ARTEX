package intercept

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseVerdict(t *testing.T) {
	for _, action := range []string{"allow", "ask", "deny"} {
		t.Run(action, func(t *testing.T) {
			reason := "Actual operation: Write a report containing the words ALLOW, DENY, and ASK; Consequence if successful: Save text without executing the commands in it; Matched rule: Custom clause"
			raw, _ := json.Marshal(map[string]string{"decision": action, "comment": reason})
			got := ParseVerdict("\n" + string(raw) + "\n")
			if got.Action != action || got.Reason != reason {
				t.Fatalf("lost verdict or explanation: %+v", got)
			}
		})
	}
}

func TestParseVerdictEnglishContract(t *testing.T) {
	for _, action := range []string{"allow", "ask", "deny"} {
		reason := "Actual operation: Read a file; Consequence if successful: Return its contents; Matched rule: A5"
		raw, _ := json.Marshal(map[string]string{"decision": action, "comment": reason})
		if got := ParseVerdict(string(raw)); got.Action != action || got.Reason != reason {
			t.Fatalf("English verdict %s: got %+v", action, got)
		}
	}
	for _, reason := range []string{
		"Actual operation: ; Consequence if successful: Return contents; Matched rule: A5",
		"Actual operation: Read a file; Consequence if successful: ; Matched rule: A5",
		"Actual operation: Read a file; Consequence if successful: Return contents; Matched rule: ",
		"Actual operation: Read a file; Matched rule: A5",
		"Actual operation: Read a file；成功后的后果：返回内容；命中规则：A5",
	} {
		raw, _ := json.Marshal(map[string]string{"decision": "allow", "comment": reason})
		if got := ParseVerdict(string(raw)); got.Action != "" {
			t.Fatalf("accepted incomplete or mixed-format verdict: %q", reason)
		}
	}
}

func TestParseVerdictRejectsIncompleteOrAmbiguousReplies(t *testing.T) {
	valid := `{"decision":"allow","comment":"Actual operation: Read a file; Consequence if successful: Return contents; Matched rule: A5"}`
	for _, reply := range []string{
		"", "ALLOW", "DENY:Matched D4", "Allowed:ALLOW", "ASK:Ownership unknown",
		`{"decision":"allow"}`, `{"decision":"approve","comment":"Actual operation: Read; Consequence if successful: Return contents; Matched rule: A5"}`,
		`{"decision":"allow","comment":null}`, `{"decision":"allow","comment":123}`,
		strings.Replace(valid, "Actual operation: Read a file", "Actual operation: ", 1),
		strings.Replace(valid, "Consequence if successful: Return contents", "Consequence if successful: ", 1),
		strings.Replace(valid, "Matched rule: A5", "Matched rule: ", 1),
		strings.Replace(valid, "; Matched rule: A5", "", 1),
		strings.Replace(valid, `"decision":"allow"`, `"decision":"deny","decision":"allow"`, 1),
		strings.Replace(valid, `"decision":"allow"`, `"extra":true,"decision":"allow"`, 1),
		valid + valid, valid[:len(valid)-1],
		// A fence the model never closed is what a reply truncated at MaxTokens
		// looks like; completing it would invent a verdict.
		"```json\n" + valid[:len(valid)-1],
		"```json\n" + valid + "\n```\nI also recommend later human review.",
		"My verdict is:\n" + valid,
	} {
		if got := ParseVerdict(reply); got.Action != "" {
			t.Errorf("accepted incomplete/ambiguous verdict: %q => %+v", reply, got)
		}
	}
}

// Wrapping JSON in markdown is the one deviation models make routinely. Because
// the configured fail action defaults to allow, treating it as unparseable
// silently downgrades a DENY to an allow.
func TestParseVerdictUnwrapsCodeFence(t *testing.T) {
	deny := `{"decision":"deny","comment":"Actual operation: Delete a production file; Consequence if successful: Business data is lost; Matched rule: D4"}`
	for _, reply := range []string{
		"```json\n" + deny + "\n```",
		"```JSON\n" + deny + "\n```",
		"```\n" + deny + "\n```",
		"  ```json\n" + deny + "\n```  ",
	} {
		got := ParseVerdict(reply)
		if got.Action != "deny" || !strings.HasSuffix(got.Reason, "Matched rule: D4") {
			t.Errorf("fenced verdict lost: %q => %+v", reply, got)
		}
	}
}

func TestParseVerdictKeepsCompleteLegacyExplanation(t *testing.T) {
	reason := "实际操作：" + strings.Repeat("Write a report", 30) + "；成功后的后果：Save the file only；命中规则：A2"
	raw, _ := json.Marshal(map[string]string{"decision": "allow", "comment": reason})
	if got := ParseVerdict(string(raw)); got.Reason != reason {
		t.Fatal("explanation was truncated or lost its rule")
	}
	raw, _ = json.Marshal(map[string]string{"decision": "allow", "comment": strings.Repeat("…", 2401)})
	if got := ParseVerdict(string(raw)); got.Action != "" {
		t.Fatal("accepted unbounded explanation")
	}
}
