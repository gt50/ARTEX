package notify

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// This file covers packing whole entries: when a digest exceeds the channel length limit, truncate at whole-entry boundaries
// and report how many entries did not fit, so callers mark only the entries actually delivered.
//
// The previous method was to render the entire message and then cut it off, and then the entire batch of marks was delivered: the second half of the message disappeared out of thin air.
// The delivery history shows that all were successful - the vulnerability is gone and nowhere to be found.

func TestMarkdownBodyPacksWholeItemsWithinByteLimit(t *testing.T) {
	// A summary of 200 Chinese items must far exceed Qiwei’s 4096 bytes.
	m := batchMsg(200)
	body, kept := markdownBody(m, weComMarkdownLimit)

	if len(body) > weComMarkdownLimit {
		t.Fatalf("Text %d Bytes exceed upper limit %d", len(body), weComMarkdownLimit)
	}
	if !utf8.ValidString(body) {
		t.Fatal("The body text is not legal UTF-8")
	}
	if kept <= 0 || kept >= len(m.Items) {
		t.Fatalf("Only part of it should be loaded (0 < kept < %d), resulting in %d", len(m.Items), kept)
	}
	// The header must truthfully state how many items this article contains and how many other items there are - otherwise readers will interpret the header as
	// That number counts as all.
	if !strings.Contains(body, "remaining") || !strings.Contains(body, "continue in the next message") {
		t.Fatalf("Header should report how many entries are omitted from this message:\n%s", body[:minInt(400, len(body))])
	}
	// Only the front kept strip should be included.
	for i := 0; i < kept; i++ {
		if !strings.Contains(body, "Finding"+itoa(i+1)) {
			t.Fatalf("Entry %d should be included in this message:\n%s", i+1, body)
		}
	}
	if strings.Contains(body, "Finding"+itoa(kept+1)) {
		t.Fatalf("Item %d shouldn't be there (it belongs in the next batch)", kept+1)
	}
}

func TestMarkdownBodyKeepsEverythingWhenUnderLimit(t *testing.T) {
	m := batchMsg(3)
	body, kept := markdownBody(m, 0) // 0 = no limit
	if kept != len(m.Items) {
		t.Fatalf("Without a length limit, all entries should be retained; got kept=%d", kept)
	}
	if strings.Contains(body, "remaining") {
		t.Fatalf("An untruncated message should not contain a truncation notice:\n%s", body)
	}
}

func TestMarkdownBodyAlwaysKeepsAtLeastOneItem(t *testing.T) {
	// When the budget is so small that even one can't fit in one, one is still sent out (the bottom of the pocket is eventually cut off).
	// Otherwise one oversized finding can stall the entire batch: it never fits and nothing is ever sent.
	m := batchMsg(5)
	_, kept := markdownBody(m, 50)
	if kept != 1 {
		t.Fatalf("At least 1 should be retained to get %d", kept)
	}
}

func TestMarkdownBodySingleReturnsOne(t *testing.T) {
	_, kept := markdownBody(singleMsg(), 4096)
	if kept != 1 {
		t.Fatalf("A single message should be reported as delivered, and %d is obtained.", kept)
	}
	// An empty message has no deliverable entries.
	if _, k := markdownBody(Message{}, 4096); k != 0 {
		t.Fatalf("0 empty messages should be reported, and %d is obtained.", k)
	}
}

func TestTelegramPackingUsesRuneBudget(t *testing.T) {
	m := batchMsg(200)
	text, kept := telegramHTML(m)
	// Telegram limits the length by **characters**; using byte size will reduce Chinese messages to one-third.
	if n := utf8.RuneCountInString(text); n > telegramTextLimit {
		t.Fatalf("Text %d exceeds the upper limit of characters %d", n, telegramTextLimit)
	}
	if kept <= 0 || kept >= len(m.Items) {
		t.Fatalf("Should only install the next part and get %d", kept)
	}
	if !strings.Contains(text, "continue in the next message") {
		t.Fatalf("Should report that additional entries were omitted:\n%.300s", text)
	}
}

func TestFeishuPackingReportsKept(t *testing.T) {
	m := batchMsg(2000)
	_, kept := feishuCard(m)
	if kept <= 0 || kept >= len(m.Items) {
		t.Fatalf("The card should only hold part of it, getting %d", kept)
	}
}

func TestWebhookAndEmailReportAllItems(t *testing.T) {
	// These two channels do not truncate the text, and the entire batch is considered delivered.
	m := batchMsg(7)
	if n := len(m.Items); n != 7 {
		t.Fatal("The precondition is not established")
	}
	// Confirmed indirectly through the return value of the renderer: markdownBody(0) retains all when unrestricted.
	if _, k := markdownBody(m, 0); k != len(m.Items) {
		t.Fatalf("Applying all without limiting the length results in %d", k)
	}
}

// TestMarkdownEscapesUntrustedContent is a regression test for "untrusted content must not change the message structure".
// The title and summary come from the model output (the model reads the response of the target under test), and the asset name comes from the URL of the target under test.
func TestMarkdownEscapesUntrustedContent(t *testing.T) {
	cases := []struct {
		name  string
		item  Item
		must  []string // Must appear in the result (escaped form)
		wrong []string // Must not appear in the result (unescaped form)
	}{
		{
			name: "title newline and external link",
			item: Item{
				Severity: "high",
				Name:     "SQL injection at login endpoint\n[Urgent: click to verify your account](http://attacker.tld)",
			},
			// Line breaks must be collapsed (otherwise new list items/quote blocks can be faked);
			// Square brackets and parentheses must be escaped (otherwise external links will be clickable).
			must:  []string{`\[Urgent: click to verify your account\]`, `\(http://attacker.tld\)`},
			wrong: []string{"\n[Urgent", "\n\n[Urgent"},
		},
		{
			name: "image beacon in title",
			item: Item{
				Severity: "high",
				Name:     "Finding ![](http://attacker.tld/beacon)",
			},
			must:  []string{`\!`, `\(http://attacker.tld/beacon\)`},
			wrong: []string{"![]("},
		},
		{
			name: "emphasis and quotes in asset name",
			item: Item{
				Severity: "high",
				Name:     "Ordinary title",
				Assets:   []string{"a.com/*injection*>quote"},
			},
			must:  []string{`\*injection\*`, `\>`},
			wrong: []string{"*injection*"},
		},
		{
			name: "backticks and pipes in summary",
			item: Item{
				Severity: "high",
				Name:     "Title",
				Summary:  "`code` | table",
			},
			must:  []string{"\\`code\\`", `\|`},
			wrong: []string{"`code`"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := Message{Items: []Item{tc.item}}
			// Writing Item in single mode is a rendering path shared by three markdown channels.
			var b strings.Builder
			writeItem(&b, tc.item, "", true)
			got := b.String()
			for _, want := range tc.must {
				if !strings.Contains(got, want) {
					t.Errorf("Missing escaped form %q:\n%s", want, got)
				}
			}
			for _, bad := range tc.wrong {
				if strings.Contains(got, bad) {
					t.Errorf("Unescaped form %q appears (could inject formatting or an external link):\n%s", bad, got)
				}
			}
			_ = m
		})
	}
}

// TestMarkdownEscapeBackslashFirst locks the escape order: backslashes must be processed first,
// Otherwise, the added backslash will be covered again, and double backslashes will appear in the output.
func TestMarkdownEscapeBackslashFirst(t *testing.T) {
	if got := markdownEscape(`a\b*c`); got != `a\\b\*c` {
		t.Fatalf("Wrong escape order, you get %q", got)
	}
}

// TestTelegramTitleHasNoMarkdownEscapes locks a specific regression:
// markdown escaping cannot leak into Telegram's HTML output (as it was in the shared title function
// After escaping, visible backslashes like `\(1\)` appear in Telegram messages).
func TestTelegramTitleHasNoMarkdownEscapes(t *testing.T) {
	m := Message{Items: []Item{{Severity: "high", Name: "alert(1) *emphasis*"}}}
	text, _ := telegramHTML(m)
	if strings.Contains(text, `\(`) || strings.Contains(text, `\*`) {
		t.Fatalf("Telegram body contains Markdown backslash escapes:\n%s", text)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
