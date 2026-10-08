package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func intPtr(n int) *int { return &n }

func TestToastDurationDefaultsToThreeSeconds(t *testing.T) {
	if got := DefaultConfig().ToastDuration(); got != 3*time.Second {
		t.Errorf("DefaultConfig toast duration = %v, want 3s", got)
	}
	if got := OneDarkConfig().ToastDuration(); got != 3*time.Second {
		t.Errorf("OneDarkConfig toast duration = %v, want 3s", got)
	}
}

func TestToastDurationFromConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		ms   *int
		want time.Duration
	}{
		{"unset falls back to default", nil, 3 * time.Second},
		{"explicit value is honored", intPtr(500), 500 * time.Millisecond},
		{"explicit zero disables", intPtr(0), 0},
		{"negative is clamped to disabled", intPtr(-100), 0},
		{"long duration", intPtr(10000), 10 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{UI: UIConfig{ToastDurationMs: tc.ms}}
			if got := c.ToastDuration(); got != tc.want {
				t.Errorf("ToastDuration() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestToastDurationNilConfigIsSafe(t *testing.T) {
	var c *Config
	if got := c.ToastDuration(); got != 3*time.Second {
		t.Errorf("nil Config ToastDuration() = %v, want 3s", got)
	}
}

func TestFillDefaultsKeepsExplicitZero(t *testing.T) {
	// A user disabling toasts must not have the default put back.
	c := &Config{UI: UIConfig{ToastDurationMs: intPtr(0)}}
	c.fillDefaults()

	if c.UI.ToastDurationMs == nil {
		t.Fatal("fillDefaults replaced an explicit 0 with nil")
	}
	if *c.UI.ToastDurationMs != 0 {
		t.Errorf("explicit 0 became %d", *c.UI.ToastDurationMs)
	}
	if got := c.ToastDuration(); got != 0 {
		t.Errorf("ToastDuration() = %v, want 0", got)
	}
}

func TestFillDefaultsSuppliesMissingDuration(t *testing.T) {
	c := &Config{}
	c.fillDefaults()

	if c.UI.ToastDurationMs == nil {
		t.Fatal("fillDefaults left toast duration unset")
	}
	if got := c.ToastDuration(); got != 3*time.Second {
		t.Errorf("ToastDuration() = %v, want 3s", got)
	}
}

// Round-tripping matters because the config file is user-editable: a config
// written by bleamd must read back with the same meaning.
func TestToastDurationJSONRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want time.Duration
	}{
		{"absent ui section", `{}`, 3 * time.Second},
		{"absent field", `{"ui":{}}`, 3 * time.Second},
		{"explicit zero", `{"ui":{"toast_duration_ms":0}}`, 0},
		{"explicit value", `{"ui":{"toast_duration_ms":1500}}`, 1500 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c Config
			if err := json.Unmarshal([]byte(tc.in), &c); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			c.fillDefaults()
			if got := c.ToastDuration(); got != tc.want {
				t.Errorf("ToastDuration() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestToastPositionDefaultsToBottomRight(t *testing.T) {
	if got := DefaultConfig().UI.ToastPosition; got != ToastBottomRight {
		t.Errorf("default position = %q, want %q", got, ToastBottomRight)
	}
	if !DefaultConfig().ToastAtBottom() {
		t.Error("DefaultConfig should anchor toasts at the bottom")
	}
	var nilCfg *Config
	if !nilCfg.ToastAtBottom() {
		t.Error("nil Config should anchor toasts at the bottom")
	}
}

func TestNormalizeToastPosition(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"top-right", ToastTopRight},
		{"TOP-RIGHT", ToastTopRight},
		{" top_right ", ToastTopRight},
		{"topright", ToastTopRight},
		{"bottom-right", ToastBottomRight},
		{"", ToastBottomRight},
		{"nonsense", ToastBottomRight},
		{"bottom-left", ToastBottomRight},
	} {
		if got := normalizeToastPosition(tc.in); got != tc.want {
			t.Errorf("normalizeToastPosition(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestToastGutterReservedOnlyForBottom(t *testing.T) {
	m := toastModel(80, 24)

	m.config.UI.ToastPosition = ToastBottomRight
	if got := m.toastGutterRows(); got != 1 {
		t.Errorf("bottom gutter = %d, want 1", got)
	}

	m.config.UI.ToastPosition = ToastTopRight
	if got := m.toastGutterRows(); got != 0 {
		t.Errorf("top gutter = %d, want 0", got)
	}
}

// alignModel builds a real model (content rendered, link hovered) so the
// status bar is showing a URL and the frame has its true layout.
func alignModel(t *testing.T, w, h int, position string) model {
	t.Helper()
	m := newModel([]byte("# Title\n\nText with a [link](https://example.com) here.\n"), false)
	m.config = DefaultConfig()
	m.config.UI.ToastPosition = position
	m.width = w
	m.height = h
	m.renderedContent = m.render()
	m = m.updateLinkPositions()
	m.hoveredURL = "https://example.com"
	return m
}

// The whole point of the bottom-right placement: the toast message must sit on
// the same screen row as the hovered URL in the status bar.
func TestBottomToastTextSharesRowWithStatusBarURL(t *testing.T) {
	m := alignModel(t, 60, 12, ToastBottomRight)
	m.toastMsg = "↗ Opening in browser…"

	lines := strings.Split(m.View(), "\n")

	urlRow, toastRow := -1, -1
	for i, l := range lines {
		plain := stripANSI(l)
		if strings.Contains(plain, "https://example.com") {
			urlRow = i
		}
		if strings.Contains(plain, "Opening in browser") {
			toastRow = i
		}
	}

	if urlRow < 0 {
		t.Fatal("status bar URL not found in frame")
	}
	if toastRow < 0 {
		t.Fatal("toast text not found in frame")
	}
	if urlRow != toastRow {
		t.Errorf("toast text on row %d but URL on row %d; they must share a row", toastRow, urlRow)
	}

	// All four borders must be present: top above, bottom below.
	if !strings.Contains(stripANSI(lines[toastRow-1]), "╭") {
		t.Errorf("no top border above the toast: %q", stripANSI(lines[toastRow-1]))
	}
	if toastRow+1 >= len(lines) {
		t.Fatal("no row below the toast for its bottom border")
	}
	if !strings.Contains(stripANSI(lines[toastRow+1]), "╰") {
		t.Errorf("no bottom border below the toast: %q", stripANSI(lines[toastRow+1]))
	}
}

func TestTopToastLeavesStatusBarOnFinalRow(t *testing.T) {
	m := alignModel(t, 60, 12, ToastTopRight)
	m.toastMsg = "↗ Opening in browser…"

	lines := strings.Split(m.View(), "\n")

	// With no gutter the status bar keeps the last row.
	if !strings.Contains(stripANSI(lines[len(lines)-1]), "https://example.com") {
		t.Errorf("status bar not on final row: %q", stripANSI(lines[len(lines)-1]))
	}
	// And the toast is up top, nowhere near it.
	if !strings.Contains(stripANSI(lines[toastMarginTop]), "╭") {
		t.Errorf("no toast top border on row %d", toastMarginTop)
	}
}

// Reserving the gutter must not change how many rows the frame occupies, or
// the terminal would scroll.
func TestFrameHeightUnchangedByPosition(t *testing.T) {
	for _, pos := range []string{ToastBottomRight, ToastTopRight} {
		for _, h := range []int{10, 24, 40} {
			m := alignModel(t, 60, h, pos)
			if got := len(strings.Split(m.View(), "\n")); got != h {
				t.Errorf("%s height=%d: frame has %d rows, want %d", pos, h, got, h)
			}
		}
	}
}

func TestShowToastSuppressedWhenDisabled(t *testing.T) {
	m := toastModel(80, 24)
	m.config.UI.ToastDurationMs = intPtr(0)

	cmd := m.showToast("should not appear")

	if cmd != nil {
		t.Error("disabled toast still scheduled a clear timer")
	}
	if m.toastMsg != "" {
		t.Errorf("disabled toast set text %q", m.toastMsg)
	}

	// And nothing is composited onto the frame.
	in := frame(80, 24)
	if got := m.renderToast(in); got != in {
		t.Error("disabled toast was rendered")
	}
}

func TestShowToastSchedulesWhenEnabled(t *testing.T) {
	m := toastModel(80, 24)
	// Short so invoking the tick below does not stall the test.
	m.config.UI.ToastDurationMs = intPtr(1)

	cmd := m.showToast("hello")

	if cmd == nil {
		t.Fatal("enabled toast did not schedule a clear timer")
	}
	if m.toastMsg != "hello" {
		t.Errorf("toastMsg = %q, want %q", m.toastMsg, "hello")
	}

	// The command must eventually yield an expiry for this toast.
	msg := cmd()
	expired, ok := msg.(toastExpiredMsg)
	if !ok {
		t.Fatalf("command returned %T, want toastExpiredMsg", msg)
	}
	if expired.seq != m.toastSeq {
		t.Errorf("expiry seq = %d, want %d", expired.seq, m.toastSeq)
	}
}
