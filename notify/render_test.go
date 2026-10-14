package notify

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateBytesKeepsValidUTF8(t *testing.T) {
	// Exercise every cut point with 1-byte ASCII, 2-byte accented Latin,
	// 3-byte punctuation, and 4-byte emoji. Never emit a partial UTF-8 rune.
	inputs := []string{
		"English text … café",
		"Mixed text — content",
		"a…b—c€d™e",
		"🔴🟠🟡🔵", // 4-byte emoji, errors are more obvious
		strings.Repeat("Finding …", 100),
	}
	for _, in := range inputs {
		for max := 1; max <= len(in)+2; max++ {
			got := TruncateBytes(in, max)
			if !utf8.ValidString(got) {
				t.Fatalf("Input %q max=%d: produces illegal UTF-8 %q", in, max, got)
			}
			if len(got) > max {
				t.Fatalf("Enter %q max=%d: the result is that %d bytes exceed the upper limit", in, max, len(got))
			}
			// Content must not be altered if it is not truncated.
			if len(in) <= max && got != in {
				t.Fatalf("Enter %q max=%d: the content is changed without exceeding the limit -> %q", in, max, got)
			}
		}
	}
}

func TestTruncateBytesZeroMeansUnlimited(t *testing.T) {
	long := strings.Repeat("x", 10000)
	if got := TruncateBytes(long, 0); got != long {
		t.Fatal("max=0 should mean no limit")
	}
	if got := TruncateBytes(long, -5); got != long {
		t.Fatal("max<0 should mean no limit")
	}
}

func TestTruncateBytesEllipsisBudget(t *testing.T) {
	// When max is less than the ellipsis itself, it cannot be exceeded by appending an ellipsis.
	got := TruncateBytes("abcdefgh", 1)
	if len(got) > 1 {
		t.Fatalf("When max=1, the result %q length %d exceeds the limit", got, len(got))
	}
	// Normally there should be an ellipsis.
	if got := TruncateBytes("abcdefgh", 5); !strings.HasSuffix(got, ellipsis) {
		t.Fatalf("Expected ellipsis, got %q", got)
	}
}

func TestTruncateRunesCountsCharactersNotBytes(t *testing.T) {
	// Telegram limits characters, while TruncateBytes limits UTF-8 bytes.
	// Three-byte punctuation makes an accidental byte-based limit observable.
	s := "…—€™…—€™…—"
	got := TruncateRunes(s, 5)
	if n := utf8.RuneCountInString(got); n != 5 {
		t.Fatalf("Expected 5 characters, got %d (%q)", n, got)
	}
	// The same string should be significantly shorter in terms of bytes.
	if utf8.RuneCountInString(TruncateBytes(s, 5)) >= 5 {
		t.Fatal("Byte caliber should not produce the same number of characters as character caliber")
	}
}

func TestOneLineCollapsesWhitespace(t *testing.T) {
	got := OneLine("First line\n\nSecond line\twith tab   extra spaces", 0)
	if strings.ContainsAny(got, "\n\t") {
		t.Fatalf("All whitespace should be collapsed, giving %q", got)
	}
	if strings.Contains(got, "  ") {
		t.Fatalf("Consecutive spaces should not be preserved, getting %q", got)
	}
	// Truncation must still be readable and legal.
	got = OneLine("…—€™…—€™…—", 4)
	if n := utf8.RuneCountInString(got); n != 4 {
		t.Fatalf("Expected 4 characters, got %d (%q)", n, got)
	}
}

func TestTruncateHTMLNeverCutsTagInHalf(t *testing.T) {
	// Directly truncating HTML will cut out fragments such as `<a href="htt`, and the platform will reject the entire message.
	s := `<b>Title</b>Body body body<a href="https://example.com/very/long/path">View details</a>`
	for max := 1; max <= utf8.RuneCountInString(s)+2; max++ {
		got := TruncateHTML(s, max)
		if n := utf8.RuneCountInString(got); max > 0 && n > max {
			t.Fatalf("max=%d: The result is %d characters exceeding the limit", max, n)
		}
		// There cannot be an unclosed `<` at the end (that is, `<` appears in the last paragraph but there is no `>`).
		if lt := strings.LastIndex(got, "<"); lt >= 0 && !strings.Contains(got[lt:], ">") {
			t.Fatalf("max=%d: The tail label is cut off -> %q", max, got)
		}
	}
}

func TestAssetLineOmitsExcess(t *testing.T) {
	if got := assetLine(nil, 3); got != "" {
		t.Fatalf("If there is no asset, an empty string should be returned and %q is obtained.", got)
	}
	if got := assetLine([]string{"a", "b"}, 3); got != "a, b" {
		t.Fatalf("All items that have not exceeded the limit should be listed, and %q is obtained.", got)
	}
	// When the upper limit is exceeded, the total number must be marked, otherwise readers will not know how many assets are not listed.
	got := assetLine([]string{"a", "b", "c", "d", "e"}, 2)
	if !strings.Contains(got, "5 total") {
		t.Fatalf("The total number should be 5, resulting in %q", got)
	}
}

func TestSeverityAndStatusLabels(t *testing.T) {
	if AtLeast("", "low") {
		t.Fatal("Empty level ordinal is 0 and should be blocked by any threshold")
	}
	if !AtLeast("critical", "") {
		t.Fatal("Empty threshold should be released")
	}
	if got := StatusLabel("fixed"); got != "Fixed" {
		t.Fatalf("Unknown state mapping, get %q", got)
	}
	// The unknown status is echoed as it is without any false labels.
	if got := StatusLabel("weird_status"); got != "weird_status" {
		t.Fatalf("The unknown status should be echoed unchanged, getting %q", got)
	}
}

// An omission pointed out by the TestTruncateHTMLNeverCutsEntity coverage audit: truncation should not only be avoided
// Half-cut tags, but also avoid cut off HTML entities.
//
// After `&amp;` is chopped into `&amp`, an entity-only parser may reject the entire message——
// However, extremely long summary messages are already common and the cost is too high.
func TestTruncateHTMLNeverCutsEntity(t *testing.T) {
	s := "aaaa&amp;bbbb&lt;cccc&quot;dddd"
	for max := 1; max <= utf8.RuneCountInString(s)+2; max++ {
		got := TruncateHTML(s, max)
		// Entity fragments "with & but no corresponding ;" must not appear at the end.
		if amp := strings.LastIndex(got, "&"); amp >= 0 && !strings.Contains(got[amp:], ";") {
			t.Fatalf("max=%d: leaving physical fragments at the tail %q", max, got[amp:])
		}
		if strings.Contains(got, "&amp\x00") {
			t.Fatalf("max=%d: Deformed entity appears", max)
		}
	}
}
