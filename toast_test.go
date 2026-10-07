package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestOverlayBoxPreservesBackground(t *testing.T) {
	bg := []string{"abcdefghij", "klmnopqrst"}
	overlayBox(bg, []string{"XX"}, 3, 0)

	if bg[0] != "abcXXfghij" {
		t.Errorf("overlay at x=3: got %q, want %q", bg[0], "abcXXfghij")
	}
	if bg[1] != "klmnopqrst" {
		t.Errorf("untouched row changed: got %q", bg[1])
	}
}

func TestOverlayBoxPadsShortBackground(t *testing.T) {
	bg := []string{"ab"}
	overlayBox(bg, []string{"XX"}, 5, 0)

	// The box must still land at column 5, padded out from the short line.
	if bg[0] != "ab   XX" {
		t.Errorf("got %q, want %q", bg[0], "ab   XX")
	}
}

func TestOverlayBoxPreservesWidthOverWideChars(t *testing.T) {
	// Background is 12 columns made of 6 double-width characters.
	const bgLine = "你好世界你好"
	if visibleWidth(bgLine) != 12 {
		t.Fatalf("precondition: background is %d columns, want 12", visibleWidth(bgLine))
	}

	// Overlay at every column; the composited line must stay 12 wide.
	for startX := 0; startX <= 8; startX++ {
		bg := []string{bgLine}
		overlayBox(bg, []string{"XXXX"}, startX, 0)

		if got := visibleWidth(bg[0]); got != 12 {
			t.Errorf("startX=%d: width %d, want 12 (%q)", startX, got, bg[0])
		}
		if !strings.Contains(bg[0], "XXXX") {
			t.Errorf("startX=%d: box missing from %q", startX, bg[0])
		}
	}
}

func TestOverlayBoxIgnoresOutOfRangeRows(t *testing.T) {
	bg := []string{"abc"}
	overlayBox(bg, []string{"X", "Y", "Z"}, 0, -1)

	// Only the row landing in range should be written.
	if len(bg) != 1 || bg[0] != "Ybc" {
		t.Errorf("got %q (len %d), want [\"Ybc\"]", bg, len(bg))
	}
}

// toastModel builds a model with just enough state to render a toast,
// avoiding LoadConfig so the test does not depend on the user's config.
func toastModel(width, height int) model {
	var m model
	m.width = width
	m.height = height
	m.config = DefaultConfig()
	m.styles.toastBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)
	return m
}

func frame(width, height int) string {
	line := strings.Repeat(".", width)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func TestRenderToastEmptyIsPassthrough(t *testing.T) {
	m := toastModel(80, 24)
	in := frame(80, 24)

	if got := m.renderToast(in); got != in {
		t.Error("empty toast modified the frame")
	}
}

func TestRenderToastAnchorsTopRightWithoutResizingFrame(t *testing.T) {
	const w, h = 80, 24
	m := toastModel(w, h)
	m.config.UI.ToastPosition = ToastTopRight
	m.toastMsg = "↗ Opening in browser…"

	in := frame(w, h)
	out := m.renderToast(in)

	inLines := strings.Split(in, "\n")
	outLines := strings.Split(out, "\n")

	if len(outLines) != len(inLines) {
		t.Fatalf("line count changed: got %d, want %d", len(outLines), len(inLines))
	}

	// Every line must keep its width, or the layout below shifts.
	for i, line := range outLines {
		if got := visibleWidth(line); got != w {
			t.Errorf("line %d width: got %d, want %d", i, got, w)
		}
	}

	// Row 0 is above the toast and must be untouched.
	if outLines[0] != inLines[0] {
		t.Error("row 0 should be above the toast")
	}

	// The toast's top border should sit on row toastMarginTop, flush to the
	// right edge minus the margin.
	row := outLines[toastMarginTop]
	if !strings.Contains(row, "╭") {
		t.Fatalf("no toast border on row %d: %q", toastMarginTop, row)
	}
	stripped := string([]rune(stripANSI(row)))
	gotStart := strings.Index(stripped, "╭")
	boxWidth := boxVisibleWidth(strings.Split(m.styles.toastBox.Render(m.toastMsg), "\n"))
	wantStart := w - boxWidth - toastMarginX
	if gotStart != wantStart {
		t.Errorf("toast starts at col %d, want %d", gotStart, wantStart)
	}

	// The message itself must survive compositing.
	if !strings.Contains(stripANSI(out), "Opening in browser") {
		t.Error("toast text missing from composited frame")
	}
}

func TestRenderToastSkippedWhenViewportTooSmall(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
	}{
		{"too narrow", 10, 24},
		{"too short", 80, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := toastModel(tc.width, tc.height)
			m.toastMsg = "↗ Opening in browser…"
			in := frame(tc.width, tc.height)

			if got := m.renderToast(in); got != in {
				t.Error("toast should be skipped rather than clipped")
			}
		})
	}
}

func TestToastSequenceGuardsAgainstStaleTimer(t *testing.T) {
	m := toastModel(80, 24)

	m.showToast("first")
	staleSeq := m.toastSeq
	m.showToast("second")

	// The first toast's timer fires after the second replaced it.
	updated, _ := m.Update(toastExpiredMsg{seq: staleSeq})
	if got := updated.(model).toastMsg; got != "second" {
		t.Errorf("stale timer cleared a newer toast: got %q, want %q", got, "second")
	}

	// The current toast's own timer does clear it.
	updated, _ = updated.(model).Update(toastExpiredMsg{seq: m.toastSeq})
	if got := updated.(model).toastMsg; got != "" {
		t.Errorf("current timer did not clear toast: got %q", got)
	}
}
