package main

import (
	"strings"
	"testing"
)

const (
	red   = "\x1b[31m"
	reset = "\x1b[0m"
	// A link whose visible label is "AB".
	osc8Link = "\x1b]8;;https://example.com\x1b\\AB\x1b]8;;\x1b\\"
)

func TestVisibleWidthCountsColumnsNotRunes(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want int
	}{
		{"ascii", "abc", 3},
		{"wide CJK is two columns each", "你好", 4},
		{"mixed", "a你b", 4},
		{"combining mark is zero width", "é", 1},
		{"ansi escapes are ignored", red + "abc" + reset, 3},
		{"osc 8 link counts only its label", osc8Link, 2},
		{"empty", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := visibleWidth(tc.in); got != tc.want {
				t.Errorf("visibleWidth(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestTruncateVisibleCharsByColumn(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"ascii exact", "abcdef", 3, "abc"},
		{"longer than input", "ab", 5, "ab"},
		{"zero", "abc", 0, ""},
		// 你 is 2 columns: 2 columns fits exactly one of them.
		{"wide char fits exactly", "你好", 2, "你"},
		// Cutting at 1 column cannot split 你, so it is dropped.
		{"wide char would straddle", "你好", 1, ""},
		{"wide after narrow", "a你", 2, "a"},
		{"wide after narrow fits", "a你", 3, "a你"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateVisibleChars(tc.in, tc.n)
			if got != tc.want {
				t.Errorf("truncateVisibleChars(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
			if w := visibleWidth(got); w > tc.n {
				t.Errorf("result width %d exceeds n=%d", w, tc.n)
			}
		})
	}
}

func TestTruncateVisibleCharsKeepsEscapes(t *testing.T) {
	got := truncateVisibleChars(red+"abcdef"+reset, 3)
	if !strings.HasPrefix(got, red) {
		t.Errorf("leading escape dropped: %q", got)
	}
	if visibleWidth(got) != 3 {
		t.Errorf("width = %d, want 3", visibleWidth(got))
	}
}

func TestSkipVisibleCharsByColumn(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"ascii", "abcdef", 3, "def"},
		{"zero skips nothing", "abc", 0, "abc"},
		{"past end", "abc", 5, ""},
		{"exactly at end", "abc", 3, ""},
		// Skipping 2 columns consumes all of 你.
		{"wide char consumed exactly", "你好", 2, "好"},
		// Column 1 is the right half of 你; it becomes a space.
		{"wide char straddles cut", "你好", 1, " 好"},
		{"narrow then wide straddles", "a你b", 2, " b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := skipVisibleChars(tc.in, tc.n)
			if got != tc.want {
				t.Errorf("skipVisibleChars(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

func TestSkipVisibleCharsCarriesEscapesForward(t *testing.T) {
	// The red set before the cut must survive, or the scrolled remainder
	// renders in the wrong color.
	got := skipVisibleChars(red+"abcdef"+reset, 3)
	if !strings.Contains(got, red) {
		t.Errorf("escape before the cut was dropped: %q", got)
	}
	if stripANSI(got) != "def" {
		t.Errorf("visible text = %q, want %q", stripANSI(got), "def")
	}
	if visibleWidth(got) != 3 {
		t.Errorf("width = %d, want 3", visibleWidth(got))
	}
}

func TestSkipVisibleCharsKeepsLinkClickableWhenCutInside(t *testing.T) {
	// Cut inside the label of a hyperlink: the remaining label must still be
	// recognized as a link, so its hit box survives horizontal scrolling.
	line := "\x1b]8;;https://example.com\x1b\\ABCDEF\x1b]8;;\x1b\\"
	got := skipVisibleChars(line, 3)

	if stripANSI(got) != "DEF" {
		t.Fatalf("visible text = %q, want %q", stripANSI(got), "DEF")
	}

	var m model
	links := m.extractLinkPositions(got)
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1 (from %q)", len(links), got)
	}
	if links[0].url != "https://example.com" {
		t.Errorf("url = %q", links[0].url)
	}
	if links[0].text != "DEF" || links[0].x != 0 || links[0].width != 3 {
		t.Errorf("link = {text:%q x:%d width:%d}, want {DEF 0 3}",
			links[0].text, links[0].x, links[0].width)
	}
}

func TestSkipVisibleCharsNoPhantomLinkFromClosedLink(t *testing.T) {
	// A complete link entirely before the cut leaves an open and a close
	// carried forward back to back. That must not register as a link.
	line := osc8Link + "tail"
	got := skipVisibleChars(line, 2)

	if stripANSI(got) != "tail" {
		t.Fatalf("visible text = %q, want %q", stripANSI(got), "tail")
	}

	var m model
	if links := m.extractLinkPositions(got); len(links) != 0 {
		t.Errorf("got %d phantom links from %q: %+v", len(links), got, links)
	}
}

// The property that overlayBox relies on: a prefix of n columns plus the
// suffix from column n must reconstruct the original width exactly, so
// composited lines never change width.
func TestTruncateAndSkipPartitionWidth(t *testing.T) {
	inputs := []string{
		"abcdef",
		"你好世界",
		"a你b好c",
		red + "ab你cd" + reset,
		osc8Link + "你",
		"éabc",
	}

	for _, in := range inputs {
		total := visibleWidth(in)
		for n := 0; n <= total; n++ {
			left := visibleWidth(truncateVisibleChars(in, n))
			right := visibleWidth(skipVisibleChars(in, n))

			// truncate may fall one column short when it drops a straddling
			// wide char; overlayBox pads that gap, so account for it here.
			pad := n - left
			if pad < 0 {
				t.Fatalf("%q n=%d: prefix %d exceeds n", in, n, left)
			}
			if got := left + pad + right; got != total {
				t.Errorf("%q n=%d: %d+%d+%d = %d, want %d",
					in, n, left, pad, right, got, total)
			}
		}
	}
}
